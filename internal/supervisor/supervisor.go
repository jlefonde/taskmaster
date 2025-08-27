package supervisor

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	s "strings"
	"syscall"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
)

func getLogFile(program *config.Program, childLogDir string, logFile string, programName string, outFile string, processNum int) string {
	if logFile != "" && s.ToUpper(logFile) != "AUTO" {
		return logFile
	}

	const charset string = "abcdefghijklmnopqrstuvwxyz0123456789"
	const suffixLen int = 8

	num := ""
	if program.NumProcs > 1 {
		num = "_" + strconv.Itoa(processNum)
	}

	suffix := make([]byte, suffixLen)
	for i := range suffixLen {
		suffix[i] = charset[rand.IntN(len(charset))]
	}

	logFileName := fmt.Sprintf("%s%s-%s---taskmaster-%s.log", programName, num, outFile, suffix)

	return filepath.Join(childLogDir, logFileName)
}

func setLogFiles(program *config.Program, childLogDir string, programName string, processNum int, cmd *exec.Cmd) error {

	if s.ToUpper(program.StdoutLogFile) != "NONE" {
		stdoutLogFile := getLogFile(program, childLogDir, program.StdoutLogFile, programName, "stdout", processNum)
		stdout, err := os.OpenFile(stdoutLogFile, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("open stdout failed: %w", err)
		}

		cmd.Stdout = stdout
	}

	if s.ToUpper(program.StderrLogFile) != "NONE" {
		stderrLogFile := getLogFile(program, childLogDir, program.StderrLogFile, programName, "stderr", processNum)
		stderr, err := os.OpenFile(stderrLogFile, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return fmt.Errorf("open stderr failed: %w", err)
		}

		cmd.Stderr = stderr
	}

	return nil
}

func cleanupLogFiles(childLogDir string) error {
	logDir, err := os.ReadDir(childLogDir)
	if err != nil {
		return fmt.Errorf("read child log directory failed: %w", err)
	} else {
		for _, entry := range logDir {
			if entry.IsDir() || !s.Contains(entry.Name(), "---taskmaster") {
				continue
			}

			if err := os.Remove(filepath.Join(childLogDir, entry.Name())); err != nil {
				log.Println("Error: remove log file failed:", err)
			}
		}
	}

	return nil
}

func Run(config *config.Config) error {
	log, err := logger.CreateLogger(config.Taskmasterd.LogFile, config.Taskmasterd.LogLevel)
	if err != nil {
		return fmt.Errorf("couldn't create logger: %w", err)
	}

	if !config.Taskmasterd.NoCleanup {
		if err := cleanupLogFiles(config.Taskmasterd.ChildLogDir); err != nil {
			log.Warning("couldn't cleanup log files:", err)
		}
	}

	fmt.Printf("taskmaster: %+v\n\n", config.Taskmasterd)
	for programName, program := range config.Programs {
		fmt.Printf("%s: %+v\n", programName, program)

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
			if err := setLogFiles(&program, config.Taskmasterd.ChildLogDir, programName, i, cmd); err != nil {
				log.Warningf("couldn't set logfile for program '%s' (process %d): %v", programName, i, err)
				continue
			}

			if err := cmd.Run(); err != nil {
				log.Warning("running process failed:", err)
			}
		}

		fmt.Println("")
	}

	return nil
}
