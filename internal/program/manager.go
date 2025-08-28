package program

import (
	"fmt"
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

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	pm := ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
	}

	return &pm
}

func (pm *ProgramManager) getDefaultLogFile(outFile string, processNum int) string {
	const charset string = "abcdefghijklmnopqrstuvwxyz0123456789"
	const suffixLen int = 8

	num := ""
	if pm.Config.NumProcs > 1 {
		num = "_" + strconv.Itoa(processNum)
	}

	suffix := make([]byte, suffixLen)
	for i := range suffixLen {
		suffix[i] = charset[rand.IntN(len(charset))]
	}

	logFileName := fmt.Sprintf("%s%s-%s---taskmaster-%s.log", pm.Name, num, outFile, suffix)

	return filepath.Join(pm.ChildLogDir, logFileName)
}

func (pm *ProgramManager) newLogFile(path string, outFile string, processNum int) (*os.File, error) {
	if s.ToUpper(path) != "NONE" {

		if path == "" || s.ToUpper(path) == "AUTO" {
			path = pm.getDefaultLogFile(outFile, processNum)
		}

		logFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("open %s failed: %w", outFile, err)
		}

		return logFile, nil
	}

	return nil, nil
}

func (pm *ProgramManager) setProcessStdLogs(cmd *exec.Cmd, processNum int) error {
	stdout, err := pm.newLogFile(pm.Config.StdoutLogFile, "stdout", processNum)
	if err != nil {
		return err
	}

	stderr, err := pm.newLogFile(pm.Config.StderrLogFile, "stderr", processNum)
	if err != nil {
		return err
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	return nil
}

func (pm *ProgramManager) Run() error {
	fmt.Printf("%s: %+v\n", pm.Name, pm.Config)

	var cmd *exec.Cmd
	if pm.Config.Umask != nil {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("umask %03o; exec %s", *pm.Config.Umask, pm.Config.Cmd))
	} else {
		cmd = exec.Command("sh", "-c", pm.Config.Cmd)
	}

	cmd.Env = os.Environ()
	for envKey, envVal := range pm.Config.Env {
		cmd.Env = append(cmd.Env, envKey+"="+envVal)
	}

	if pm.Config.WorkingDir != "" {
		cmd.Dir = pm.Config.WorkingDir
	}

	user, err := user.Lookup(pm.Config.User)
	if err != nil {
		return fmt.Errorf("user lookup failed: %w", err)
	}

	uid, _ := strconv.ParseInt(user.Uid, 10, 32)
	gid, _ := strconv.ParseInt(user.Gid, 10, 32)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	for processNum := range pm.Config.NumProcs {
		if err := pm.setProcessStdLogs(cmd, processNum); err != nil {
			pm.Log.Warningf("couldn't set logfile for program '%s' (process %d): %v", pm.Name, processNum, err)
			continue
		}

		if err := cmd.Run(); err != nil {
			pm.Log.Warning("running process failed:", err)
		}
	}

	fmt.Println("")

	return nil
}
