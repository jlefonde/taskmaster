package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

type AutoRestart string

const (
	AUTORESTART_NEVER      AutoRestart = "never"
	AUTORESTART_ALWAYS     AutoRestart = "always"
	AUTORESTART_ON_FAILURE AutoRestart = "on-failure"
	AUTORESTART_UNKNOWN    AutoRestart = "unknown"
)

type Program struct {
	Cmd           string            `mapstructure:"cmd"`
	NumProcs      int               `mapstructure:"numprocs"`
	Umask         int               `mapstructure:"umask"`
	WorkingDir    string            `mapstructure:"workingdir"`
	User          string            `mapstructure:"user"`
	AutoStart     bool              `mapstructure:"autostart"`
	AutoRestart   AutoRestart       `mapstructure:"autorestart"`
	ExitCodes     []int             `mapstructure:"exitcodes"`
	StartRetries  int               `mapstructure:"startretries"`
	StartSecs     int               `mapstructure:"startsecs"`
	StopSignal    syscall.Signal    `mapstructure:"stopsignal"`
	StopSecs      int               `mapstructure:"stopsecs"`
	StdoutLogfile string            `mapstructure:"stdout_logfile"`
	StderrLogfile string            `mapstructure:"stderr_logfile"`
	Env           map[string]string `mapstructure:"env"`
}

func validateProgram(program Program) error {
	if program.Cmd == "" {
		return errors.New("cmd required")
	}
	return nil
}

func defaultsHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		// fmt.Printf("f: %+v, t: %+v, data: %+v\n", f, t, data)
		if t == reflect.TypeOf(Program{}) {
			program := Program{
				NumProcs:      1,
				AutoStart:     true,
				AutoRestart:   AUTORESTART_ON_FAILURE,
				ExitCodes:     []int{0},
				StartRetries:  3,
				StartSecs:     1,
				StopSignal:    15,
				StopSecs:      10,
				StdoutLogfile: "/tmp/stdout.log",
				StderrLogfile: "/tmp/stderr.log",
				Env:           make(map[string]string),
			}

			programMap := data.(map[string]any)

			autoRestart, autoRestartFound := programMap["autorestart"]
			if autoRestartFound {
				autoRestartStr, ok := autoRestart.(string)
				if !ok {
					program.AutoRestart = AUTORESTART_UNKNOWN
				} else {
					switch autoRestartStr {
					case "never":
						program.AutoRestart = AUTORESTART_NEVER
					case "always":
						program.AutoRestart = AUTORESTART_ALWAYS
					case "on-failure":
						program.AutoRestart = AUTORESTART_ON_FAILURE
					default:
						program.AutoRestart = AUTORESTART_UNKNOWN
					}
				}
				delete(programMap, "autorestart")
			}

			decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				WeaklyTypedInput: true,
				Result:           &program,
			})

			decoder.Decode(programMap)
			return program, nil
		}
		return data, nil
	}
}

func readConfigFile(configPath string) ([]byte, error) {
	if configPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		executablePath, err := os.Executable()
		if err != nil {
			return nil, err
		}

		dir := filepath.Dir(executablePath)

		defaultLocations := []string{
			filepath.Join(dir, "../etc/taskmasterd.yaml"),
			filepath.Join(dir, "../taskmasterd.yaml"),
			filepath.Join(cwd, "/taskmasterd.yaml"),
			filepath.Join(cwd, "/etc/taskmasterd.yaml"),
			"/etc/taskmasterd.yaml",
			"/etc/taskmaster/taskmasterd.conf",
		}

		var conf []byte
		for _, location := range defaultLocations {
			conf, err = os.ReadFile(location)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				} else {
					return nil, err
				}
			}
			break
		}

		if conf == nil {
			return nil, errors.New("no configuration file found in default locations")
		}

		return conf, nil
	}

	conf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

func Parse(configPath string) (map[string]Program, error) {
	conf, err := readConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	var raw any
	if err := yaml.Unmarshal(conf, &raw); err != nil {
		return nil, err
	}

	configMap, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("configuration file is not a valid YAML")
	}

	rawPrograms, programsFound := configMap["programs"]
	if !programsFound {
		return nil, errors.New("no 'programs' section found in configuration")
	}

	var programs map[string]Program
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &programs, WeaklyTypedInput: true, DecodeHook: defaultsHook()})
	if err := decoder.Decode(rawPrograms); err != nil {
		return nil, err
	}

	for _, program := range programs {
		err := validateProgram(program)
		if err != nil {
			return nil, err
		}
	}

	return programs, nil
}
