package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jlefonde/taskmaster/internal/config"
	"github.com/jlefonde/taskmaster/internal/supervisor"
)

var ctx config.Context

func init() {
	if euid := os.Geteuid(); euid != 0 {
		fmt.Fprintln(os.Stderr, "Error: can't drop privileges as nonroot user")
		os.Exit(1)
	}

	const (
		configDefault      = ""
		configUsage        = "The path to a taskmasterd configuration file."
		noCleanupDefault   = false
		noCleanupUsage     = "Prevent taskmasterd from performing cleanup (removal of old AUTO process log files) at startup."
		childLogDirDefault = ""
		childLogDirUsage   = "A path to a directory (it must already exist) where taskmaster will write its AUTO -mode child process logs."
		logFileDefault     = ""
		logFileUsage       = "The path to the taskmasterd activity log."
		logLevelDefault    = ""
		logLevelUsage      = "The logging level at which taskmaster should write to the activity log. [DEBUG, INFO, WARNING, ERROR, CRITICAL]"
	)

	flag.StringVar(&ctx.ConfigPath, "configuration", configDefault, configUsage)
	flag.StringVar(&ctx.ConfigPath, "c", configDefault, configUsage)
	flag.BoolVar(&ctx.NoCleanup, "nocleanup", noCleanupDefault, noCleanupUsage)
	flag.BoolVar(&ctx.NoCleanup, "k", noCleanupDefault, noCleanupUsage)
	flag.StringVar(&ctx.ChildLogDir, "childlogdir", childLogDirDefault, childLogDirUsage)
	flag.StringVar(&ctx.ChildLogDir, "q", childLogDirDefault, childLogDirUsage)
	flag.StringVar(&ctx.LogFile, "logfile", logFileDefault, logFileUsage)
	flag.StringVar(&ctx.LogFile, "l", logFileDefault, logFileUsage)
	flag.StringVar(&ctx.LogLevel, "loglevel", logLevelDefault, logLevelUsage)
	flag.StringVar(&ctx.LogLevel, "e", logLevelDefault, logLevelUsage)
}

func main() {
	flag.Parse()

	config, err := config.NewConfigWithContext(&ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to create config:", err)
		os.Exit(1)
	}

	supervisor, err := supervisor.NewSupervisor(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to create supervisor:", err)
		os.Exit(1)
	}

	supervisor.Run()
}
