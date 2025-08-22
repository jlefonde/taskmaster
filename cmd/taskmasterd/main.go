package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"reflect"

	"taskmaster/internal/config"
	"taskmaster/internal/version"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

type Context struct {
	configPath string
}

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
	StopSignal    int               `mapstructure:"stopsignal"`
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

var ctx Context

func init() {
	const (
		configDefault = ""
		configUsage   = "The path to a taskmasterd configuration file."
		versionUsage  = "Print the taskmasterd version number out to stdout and exit."
	)

	flag.BoolFunc("version", versionUsage, version.PrintVersion)
	flag.BoolFunc("v", versionUsage, version.PrintVersion)
	flag.StringVar(&ctx.configPath, "configuration", configDefault, configUsage)
	flag.StringVar(&ctx.configPath, "c", configDefault, configUsage)
}

func main() {
	flag.Parse()

	conf, err := config.ReadConfigFile(ctx.configPath)
	if err != nil {
		log.Fatal("Error: couldn't read the configuration file: ", err)
	}

	var raw any
	if err := yaml.Unmarshal(conf, &raw); err != nil {
		log.Fatal(err)
	}

	configMap, ok := raw.(map[string]any)
	if !ok {
		log.Fatal("Error: configuration file is not a valid YAML")
	}

	rawPrograms, programsFound := configMap["programs"]
	if programsFound {
		var programs map[string]Program

		decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result: &programs, WeaklyTypedInput: true, DecodeHook: defaultsHook()})
		if err := decoder.Decode(rawPrograms); err != nil {
			log.Fatal(err)
		}

		for _, program := range programs {
			fmt.Printf("%+v\n", program)
			err := validateProgram(program)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}
