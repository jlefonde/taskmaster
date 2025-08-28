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
	"time"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
)

type ManagedProcess struct {
	Name    string
	Process *os.Process
}

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
	Processes   map[int]*ManagedProcess
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	pm := ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
		Processes:   make(map[int]*ManagedProcess),
	}

	return &pm
}

func (pm *ProgramManager) getDefaultLogFile(outFile string, processNum int) string {
	const charset string = "abcdefghijklmnopqrstuvwxyz0123456789"
	const suffixLen int = 8

	num := ""
	if pm.Config.NumProcs > 1 {
		num = "_" + fmt.Sprintf("%02d", processNum)
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

func (pm *ProgramManager) getProcessName(processNum int) string {
	if pm.Config.NumProcs == 1 {
		return pm.Name
	}

	return fmt.Sprintf("%s:%s_%02d", pm.Name, pm.Name, processNum)
}

func (pm *ProgramManager) prepareCmd(processNum int) (*exec.Cmd, error) {
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

	cmd.Dir = pm.Config.WorkingDir

	user, err := user.Lookup(pm.Config.User)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	uid, _ := strconv.ParseInt(user.Uid, 10, 32)
	gid, _ := strconv.ParseInt(user.Gid, 10, 32)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	if err := pm.setProcessStdLogs(cmd, processNum); err != nil {
		return nil, fmt.Errorf("couldn't set logfile: %w", err)
	}

	return cmd, nil
}

func (pm *ProgramManager) Run() error {
	fmt.Printf("%s: %+v\n\n", pm.Name, pm.Config)

	exited := make(chan error, 1)
	for processNum := range pm.Config.NumProcs {
		cmd, err := pm.prepareCmd(processNum)
		if err != nil {
			pm.Log.Warningf("prepare cmd failed for program '%s' (process %d): %v", pm.Name, processNum, err)
			continue
		}

		if err := cmd.Start(); err != nil {
			continue
		}

		pm.Processes[cmd.Process.Pid] = &ManagedProcess{
			Name:    pm.getProcessName(processNum),
			Process: cmd.Process,
		}

		go func(cmd *exec.Cmd) {
			exited <- cmd.Wait()
		}(cmd)

		fmt.Printf("process_%02d: %+v\n", processNum, pm.Processes[cmd.Process.Pid])
	}

	for {
		time.Sleep(10 * time.Second)
	}

	// fmt.Println("")

	return nil
}
