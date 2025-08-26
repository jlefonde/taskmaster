package supervisor

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	s "strings"
	"syscall"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
)

func getLogFile(LogFile string, programName string, outFile string, processNum int, numProcs int) string {
	if LogFile != "" {
		return LogFile
	}

	const charset string = "abcdefghijklmnopqrstuvwxyz0123456789"
	const suffixLen int = 8

	suffix := make([]byte, suffixLen)
	for i := range suffixLen {
		suffix[i] = charset[rand.IntN(len(charset))]
	}

	childLogDir := "/var/log/"
	num := ""
	if numProcs > 1 {
		num = "_" + strconv.Itoa(processNum)
	}

	return fmt.Sprintf("%s%s%s-%s---taskmaster-%s.log", childLogDir, programName, num, outFile, suffix)
}

func setLogFiles(programName string, program config.Program, processNum int, cmd *exec.Cmd) error {
	stdoutLogFile := getLogFile(program.StdoutLogFile, programName, "stdout", processNum, program.NumProcs)
	stdout, err := os.OpenFile(stdoutLogFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open stdout failed: %w", err)
	}

	stderrLogFile := getLogFile(program.StderrLogFile, programName, "stderr", processNum, program.NumProcs)
	stderr, err := os.OpenFile(stderrLogFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open stderr failed: %w", err)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return nil
}

func cleanupLogFiles() error {
	logDir, err := os.ReadDir("/var/log/")
	if err != nil {
		return fmt.Errorf("read child log directory failed: %w", err)
	} else {
		for _, entry := range logDir {
			if entry.IsDir() || !s.Contains(entry.Name(), "---taskmaster") {
				continue
			}

			if err := os.Remove("/var/log/" + entry.Name()); err != nil {
				log.Println("Error: remove log file failed:", err)
			}
		}
	}

	return nil
}

func Run(config *config.Config) {
	log, err := logger.CreateLogger("/var/log/taskmasterd.log", logger.ERROR)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: couldn't create logger:", err)
		os.Exit(1)
	}

	if err := cleanupLogFiles(); err != nil {
		log.Warning("couldn't cleanup log files:", err)
	}

	for programName, program := range config.Programs {
		fmt.Printf("%+v\n", program)

		cmd := exec.Command("sh", "-c", fmt.Sprintf("umask %03o; exec %s", program.Umask, program.Cmd))
		cmd.Env = os.Environ()

		for envKey, envVal := range program.Env {
			cmd.Env = append(cmd.Env, envKey+"="+envVal)
		}

		if program.WorkingDir != "" {
			cmd.Dir = program.WorkingDir
		}

		user, err := user.Lookup(program.User)
		if err != nil {
			log.Error("user lookup failed:", err)
			continue
		}

		uid, _ := strconv.ParseInt(user.Uid, 10, 32)
		gid, _ := strconv.ParseInt(user.Gid, 10, 32)

		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
		for i := range program.NumProcs {
			if err := setLogFiles(programName, program, i, cmd); err != nil {
				log.Warningf("couldn't set LogFile for program '%s' (process %d): %v", programName, i, err)
				continue
			}

			if err := cmd.Run(); err != nil {
				log.Warning("running process failed:", err)
			}
		}
	}
}
