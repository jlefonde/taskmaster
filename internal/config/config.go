package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	s "strings"
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

func checkBaseDirExists(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("the directory named as part of the path '%s' does not exist", path)
	}

	return nil
}

func checkDirExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("the path '%s' does not exist", path)
	}

	if !info.IsDir() {
		return fmt.Errorf("the path '%s' is not a directory", path)
	}

	return nil
}

func validateProgram(programName string, program Program) error {
	if program.Cmd == "" {
		return errors.New("'cmd' required")
	}

	if program.NumProcs > 1 && !s.Contains(programName, "%(process_num)") {
		return errors.New("%(process_num) must be present within program_name when numprocs > 1")
	}

	if program.AutoRestart == AUTORESTART_UNKNOWN {
		return errors.New("invalid 'autorestart'")
	}

	if program.StopSignal < 0 || program.StopSignal > 34 {
		return errors.New("invalid 'stopsignal'")
	}

	for _, val := range program.ExitCodes {
		if val < 0 || val > 255 {
			return errors.New("invalid 'exitcodes'")
		}
	}

	if _, err := user.Lookup(program.User); err != nil {
		return errors.New("invalid 'user'")
	}

	if err := checkDirExists(program.WorkingDir); err != nil {
		return err
	}

	if err := checkBaseDirExists(program.StdoutLogfile); err != nil {
		return err
	}

	if err := checkBaseDirExists(program.StderrLogfile); err != nil {
		return err
	}

	return nil
}

func parseAutoRestart(programMap map[string]any, program *Program) {
	autoRestart, autoRestartFound := programMap["autorestart"]
	if !autoRestartFound {
		return
	}

	program.AutoRestart = AUTORESTART_UNKNOWN

	autoRestartStr, ok := autoRestart.(string)
	if !ok {
		return
	}

	autoRestartMap := map[string]AutoRestart{
		"never":      AUTORESTART_NEVER,
		"always":     AUTORESTART_ALWAYS,
		"on-failure": AUTORESTART_ON_FAILURE,
	}

	if val, found := autoRestartMap[autoRestartStr]; found {
		program.AutoRestart = val
	}

	delete(programMap, "autorestart")
}

func parseStopSignal(programMap map[string]any, program *Program) {
	stopSignal, stopSignalFound := programMap["stopsignal"]
	if !stopSignalFound {
		return
	}

	stopSignalStr, ok := stopSignal.(string)
	if !ok {
		return
	}

	signalMap := map[string]syscall.Signal{
		"ABRT":   syscall.SIGABRT,
		"ALRM":   syscall.SIGALRM,
		"BUS":    syscall.SIGBUS,
		"CHLD":   syscall.SIGCHLD,
		"CLD":    syscall.SIGCLD,
		"CONT":   syscall.SIGCONT,
		"FPE":    syscall.SIGFPE,
		"HUP":    syscall.SIGHUP,
		"ILL":    syscall.SIGILL,
		"INT":    syscall.SIGINT,
		"IO":     syscall.SIGIO,
		"IOT":    syscall.SIGIOT,
		"KILL":   syscall.SIGKILL,
		"PIPE":   syscall.SIGPIPE,
		"POLL":   syscall.SIGPOLL,
		"PROF":   syscall.SIGPROF,
		"PWR":    syscall.SIGPWR,
		"QUIT":   syscall.SIGQUIT,
		"SEGV":   syscall.SIGSEGV,
		"STKFLT": syscall.SIGSTKFLT,
		"STOP":   syscall.SIGSTOP,
		"SYS":    syscall.SIGSYS,
		"TERM":   syscall.SIGTERM,
		"TRAP":   syscall.SIGTRAP,
		"TSTP":   syscall.SIGTSTP,
		"TTIN":   syscall.SIGTTIN,
		"TTOU":   syscall.SIGTTOU,
		"UNUSED": syscall.SIGUNUSED,
		"URG":    syscall.SIGURG,
		"USR1":   syscall.SIGUSR1,
		"USR2":   syscall.SIGUSR2,
		"VTALRM": syscall.SIGVTALRM,
		"WINCH":  syscall.SIGWINCH,
		"XCPU":   syscall.SIGXCPU,
		"XFSZ":   syscall.SIGXFSZ,
	}

	stopSignalStr = s.ToUpper(stopSignalStr)
	stopSignalStr, _ = s.CutPrefix(stopSignalStr, "SIG")

	if sig, found := signalMap[stopSignalStr]; found {
		program.StopSignal = sig
	} else {
		program.StopSignal = -1
	}

	delete(programMap, "stopsignal")
}

func defaultsHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if t == reflect.TypeOf(Program{}) {
			program := Program{
				NumProcs:     1,
				WorkingDir:   "/",
				User:         "root",
				AutoStart:    true,
				AutoRestart:  AUTORESTART_ON_FAILURE,
				ExitCodes:    []int{0},
				StartRetries: 3,
				StartSecs:    1,
				StopSignal:   15,
				StopSecs:     10,
				Env:          make(map[string]string),
			}

			programMap := data.(map[string]any)
			parseAutoRestart(programMap, &program)
			parseStopSignal(programMap, &program)

			decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				WeaklyTypedInput: true,
				Result:           &program,
			})

			if err := decoder.Decode(programMap); err != nil {
				return nil, err
			}

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
			defaultLocationsStr := "(" + s.Join(defaultLocations, ", ") + ")"
			errorMsg := s.Join([]string{"no configuration file found at default locations", defaultLocationsStr}, " ")
			return nil, errors.New(errorMsg)
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
		return nil, errors.New("no 'programs' section found in configuration file")
	}

	var programs map[string]Program
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: &programs, WeaklyTypedInput: true, DecodeHook: defaultsHook()})
	if err := decoder.Decode(rawPrograms); err != nil {
		return nil, err
	}

	for programName, program := range programs {
		err := validateProgram(programName, program)
		if err != nil {
			return nil, fmt.Errorf("%w in section 'programs:%s' (file: '%s')", err, programName, configPath)
		}
	}

	return programs, nil
}
