package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	s "strings"
	"syscall"

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
	if euid := os.Geteuid(); euid != 0 {
		log.Fatal("Error: can't drop privileges as nonroot user")
	}

	flag.Parse()

	programs, err := config.Parse(ctx.configPath)
	if err != nil {
		log.Fatal("Error: parse config failed: ", err)
	}

	for _, program := range programs {
		fmt.Printf("%+v\n", program)

		cmd := exec.Command("sh", "-c", fmt.Sprintf("umask %03o; exec %s", program.Umask, program.Cmd))
		cmd.Env = os.Environ()

		for envKey, envVal := range program.Env {
			cmd.Env = append(cmd.Env, s.Join([]string{envKey, envVal}, "="))
		}

		if program.WorkingDir != "" {
			cmd.Dir = program.WorkingDir
		}

		user, err := user.Lookup(program.User)
		if err != nil {
			log.Println("Error: user lookup failed:", err)
			continue
		}

		uid, _ := strconv.ParseInt(user.Uid, 10, 32)
		gid, _ := strconv.ParseInt(user.Gid, 10, 32)

		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		for i := 0; i < program.NumProcs; i++ {
			// stdout, err := os.OpenFile(program.StdoutLogfile, os.O_CREATE|os.O_RDWR, 0644)
			// if err != nil {
			// 	log.Println("Error: open stdout failed:", err)
			// 	continue
			// }

			// stderr, err := os.OpenFile(program.StderrLogfile, os.O_CREATE|os.O_RDWR, 0644)
			// if err != nil {
			// 	log.Println("Error: open stderr failed:", err)
			// 	continue
			// }

			// cmd.Stdout = stdout
			// cmd.Stderr = stderr

			if err := cmd.Run(); err != nil {
				log.Println("Error: running process failed:", err)
			}
		}
	}
}
