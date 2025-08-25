package main

import (
	"flag"
	"fmt"
	"log"

	"taskmaster/internal/config"
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
	// if euid := os.Geteuid(); euid != 0 {
	// 	log.Fatal("Error: can't drop privileges as nonroot user")
	// }

	flag.Parse()

	programs, err := config.Parse(ctx.configPath)
	if err != nil {
		log.Fatal("Error: parse config failed: ", err)
	}

	for _, program := range programs {
		fmt.Printf("%+v\n", program)
	}
}
