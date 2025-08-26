package main

import (
	"flag"
	"fmt"
	"os"

	"taskmaster/internal/config"
	"taskmaster/internal/supervisor"
	"taskmaster/internal/version"
)

type Context struct {
	configPath string
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
	if euid := os.Geteuid(); euid != 0 {
		fmt.Fprintln(os.Stderr, "Error: can't drop privileges as nonroot user")
		os.Exit(1)
	}

	flag.Parse()

	programs, err := config.Parse(ctx.configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to parse configuration file:", err)
		os.Exit(1)
	}

	supervisor.Run(programs)
}
