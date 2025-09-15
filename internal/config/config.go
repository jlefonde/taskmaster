package config

import (
	"errors"
	"fmt"
	"log/syslog"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"taskmaster/internal/logger"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

type Context struct {
	ConfigPath  string
	NoDaemon    bool
	NoCleanup   bool
	ChildLogDir string
	LogFile     string
	LogLevel    string
}

type AutoRestart string

const (
	AUTORESTART_NEVER      AutoRestart = "never"
	AUTORESTART_ALWAYS     AutoRestart = "always"
	AUTORESTART_ON_FAILURE AutoRestart = "on-failure"
	AUTORESTART_UNKNOWN    AutoRestart = "unknown"
)

type Taskmasterd struct {
	NoDaemon    bool            `mapstructure:"nodaemon"`
	NoCleanup   bool            `mapstructure:"nocleanup"`
	ChildLogDir string          `mapstructure:"childlogdir"`
	LogFile     string          `mapstructure:"logfile"`
	LogLevel    syslog.Priority `mapstructure:"loglevel"`
}

type Program struct {
	Cmd           string            `mapstructure:"cmd"`
	NumProcs      int               `mapstructure:"numprocs"`
	Umask         *int              `mapstructure:"umask"`
	WorkingDir    string            `mapstructure:"workingdir"`
	User          string            `mapstructure:"user"`
	AutoStart     bool              `mapstructure:"autostart"`
	AutoRestart   AutoRestart       `mapstructure:"autorestart"`
	ExitCodes     []int             `mapstructure:"exitcodes"`
	StartRetries  int               `mapstructure:"startretries"`
	StartSecs     int               `mapstructure:"startsecs"`
	StopSignal    syscall.Signal    `mapstructure:"stopsignal"`
	StopSecs      int               `mapstructure:"stopsecs"`
	StdoutLogFile string            `mapstructure:"stdout_logfile"`
	StderrLogFile string            `mapstructure:"stderr_logfile"`
	Env           map[string]string `mapstructure:"env"`
}

type Config struct {
	Path        string
	Taskmasterd Taskmasterd
	Programs    map[string]Program
}

func checkBaseDirExists(path string) error {
	if path == "" {
		return nil
	}

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("the directory named as part of the path '%s' does not exist", path)
	}

	return nil
}

func checkFilePath(path string) error {
	if path == "" {
		return nil
	}

	if err := checkBaseDirExists(path); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("the path '%s' exists but is not a regular file", path)
	}

	return nil
}

func checkDirExists(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("the path '%s' does not exist", path)
	}

	if !info.IsDir() {
		return fmt.Errorf("the path '%s' is not a directory", path)
	}

	return nil
}

func validateTaskmasterd(taskmasterd Taskmasterd) error {
	if err := checkDirExists(taskmasterd.ChildLogDir); err != nil {
		return err
	}

	if taskmasterd.LogFile != logger.SYSLOG {
		if err := checkFilePath(taskmasterd.LogFile); err != nil {
			return err
		}
	}

	if taskmasterd.LogLevel == logger.LOG_UNKNOWN {
		return errors.New("invalid 'loglevel'")
	}

	return nil
}

func validateProgram(program Program) error {
	if program.Cmd == "" {
		return errors.New("'cmd' required")
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

	if err := checkFilePath(program.StdoutLogFile); err != nil {
		return err
	}

	if err := checkFilePath(program.StderrLogFile); err != nil {
		return err
	}

	return nil
}

func getLogLevelFromStr(logLevelStr string) (syslog.Priority, error) {
	logLevelMap := map[string]syslog.Priority{
		"critical": syslog.LOG_CRIT,
		"error":    syslog.LOG_ERR,
		"warning":  syslog.LOG_WARNING,
		"info":     syslog.LOG_INFO,
		"debug":    syslog.LOG_DEBUG,
	}

	if val, found := logLevelMap[strings.ToLower(logLevelStr)]; found {
		return val, nil
	}

	return logger.LOG_UNKNOWN, errors.New("invalid 'loglevel'")
}

func parseLogLevel(taskmasterdMap map[string]any, taskmasterd *Taskmasterd) {
	logLevel, logLevelFound := taskmasterdMap["loglevel"]
	if !logLevelFound {
		return
	}

	logLevelStr, ok := logLevel.(string)
	if !ok {
		return
	}

	taskmasterd.LogLevel, _ = getLogLevelFromStr(logLevelStr)

	delete(taskmasterdMap, "loglevel")
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

	if val, found := autoRestartMap[strings.ToLower(autoRestartStr)]; found {
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

	stopSignalStr = strings.ToUpper(stopSignalStr)
	stopSignalStr, _ = strings.CutPrefix(stopSignalStr, "SIG")

	if sig, found := signalMap[stopSignalStr]; found {
		program.StopSignal = sig
	} else {
		program.StopSignal = -1
	}

	delete(programMap, "stopsignal")
}

func getDefaultTaskmasterd() (*Taskmasterd, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	taskmasterd := Taskmasterd{
		NoDaemon:    false,
		NoCleanup:   false,
		ChildLogDir: os.TempDir(),
		LogFile:     cwd + "/taskmasterd.log",
		LogLevel:    syslog.LOG_INFO,
	}

	return &taskmasterd, nil
}

func taskmasterdHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if t == reflect.TypeOf(Taskmasterd{}) {
			taskmasterd, err := getDefaultTaskmasterd()
			if err != nil {
				return nil, err
			}

			taskmasterdMap := data.(map[string]any)
			parseLogLevel(taskmasterdMap, taskmasterd)

			decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
				WeaklyTypedInput: true,
				Result:           &taskmasterd,
			})

			if err := decoder.Decode(taskmasterdMap); err != nil {
				return nil, err
			}

			return taskmasterd, nil
		}

		return data, nil
	}
}

func programsHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if t == reflect.TypeOf(Program{}) {
			program := Program{
				NumProcs:     1,
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
			"/etc/taskmaster/taskmasterd.yaml",
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

		if len(conf) == 0 {
			defaultLocationsStr := "(" + strings.Join(defaultLocations, ", ") + ")"
			return nil, fmt.Errorf("no configuration file found at default locations %s", defaultLocationsStr)
		}

		return conf, nil
	}

	conf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

func decodeRawMap(configMap map[string]any, key string, result any, decodeHook mapstructure.DecodeHookFunc) error {
	raw, found := configMap[key]
	if found {
		decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result: result, WeaklyTypedInput: true, DecodeHook: decodeHook})
		if err := decoder.Decode(raw); err != nil {
			return err
		}
	}

	return nil
}

func (config *Config) parseTaskmasterd(configMap map[string]any) error {
	if err := decodeRawMap(configMap, "taskmasterd", &config.Taskmasterd, taskmasterdHook()); err != nil {
		return err
	}

	if err := validateTaskmasterd(config.Taskmasterd); err != nil {
		return fmt.Errorf("%w in section 'taskmasterd' (file: '%s')", err, config.Path)
	}

	return nil
}

func (config *Config) parsePrograms(configMap map[string]any) error {
	if err := decodeRawMap(configMap, "programs", &config.Programs, programsHook()); err != nil {
		return err
	}

	for programName, program := range config.Programs {
		err := validateProgram(program)
		if err != nil {
			return fmt.Errorf("%w in section 'programs:%s' (file: '%s')", err, programName, config.Path)
		}
	}

	return nil
}

func newTaskmasterd(ctx *Context) (*Taskmasterd, error) {
	taskmasterd := Taskmasterd{
		NoDaemon:    ctx.NoDaemon,
		NoCleanup:   ctx.NoCleanup,
		ChildLogDir: ctx.ChildLogDir,
		LogFile:     ctx.LogFile,
		LogLevel:    logger.LOG_NONE,
	}

	if ctx.LogLevel != "" {
		logLevel, err := getLogLevelFromStr(ctx.LogLevel)
		if err != nil {
			return nil, err
		}

		taskmasterd.LogLevel = logLevel
	}

	if err := validateTaskmasterd(taskmasterd); err != nil {
		return nil, err
	}

	return &taskmasterd, nil
}

func (config *Config) overrideTaskmasterd(override *Taskmasterd) {
	base := &config.Taskmasterd

	if override.NoDaemon {
		base.NoDaemon = override.NoDaemon
	}

	if override.NoCleanup {
		base.NoCleanup = override.NoCleanup
	}

	if override.ChildLogDir != "" {
		base.ChildLogDir = override.ChildLogDir
	}

	if override.LogFile != "" {
		base.LogFile = override.LogFile
	}

	if override.LogLevel != logger.LOG_NONE {
		base.LogLevel = override.LogLevel
	}
}

func (config *Config) parse() error {
	conf, err := readConfigFile(config.Path)
	if err != nil {
		return err
	}

	var raw any
	if err := yaml.Unmarshal(conf, &raw); err != nil {
		return err
	}

	configMap, ok := raw.(map[string]any)
	if !ok {
		return errors.New("configuration file is not a valid YAML")
	}

	if err := config.parseTaskmasterd(configMap); err != nil {
		return err
	}

	if err := config.parsePrograms(configMap); err != nil {
		return err
	}

	return nil
}

func NewConfig(configPath string) (*Config, error) {
	config := Config{
		Path: configPath,
	}

	if err := config.parse(); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	return &config, nil
}

func NewConfigWithContext(ctx *Context) (*Config, error) {
	taskmasterd, err := newTaskmasterd(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration flags: %w", err)
	}

	config, err := NewConfig(ctx.ConfigPath)
	if err != nil {
		return nil, err
	}

	config.overrideTaskmasterd(taskmasterd)

	return config, nil
}
