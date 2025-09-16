package program

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jlefonde/taskmaster/internal/config"
)

type ProcessExitInfo struct {
	Mp       *ManagedProcess
	ExitTime time.Time
	Err      error
}

type RequestReply struct {
	Name    string
	Message string
	Err     error
}

type ProcessStatus struct {
	Name        string
	State       State
	Description string
	Err         error
}

type ManagedProcess struct {
	Name            string
	Num             int
	Cmd             *exec.Cmd
	State           State
	StartTime       time.Time
	StopTime        time.Time
	ExitTime        time.Time
	NextRestartTime time.Time
	StoppedSignal   os.Signal
	RestartCount    int
	ExitChan        chan ProcessExitInfo
	Stdout          *os.File
	Stderr          *os.File
	StdoutLogFile   string
	StderrLogFile   string
	mu              sync.Mutex
}

func newManagedProcess(processNum int, processName string, exitChan chan ProcessExitInfo, stoppedSignal os.Signal) *ManagedProcess {
	return &ManagedProcess{
		Name:          processName,
		Num:           processNum,
		State:         STOPPED,
		ExitChan:      exitChan,
		StoppedSignal: stoppedSignal,
	}
}

func (mp *ManagedProcess) getCmd() *exec.Cmd {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.Cmd
}

func (mp *ManagedProcess) setCmd(cmd *exec.Cmd) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.Cmd = cmd
}

func (mp *ManagedProcess) getState() State {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.State
}

func (mp *ManagedProcess) setState(state State) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.State = state
}

func (mp *ManagedProcess) getStartTime() time.Time {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.StartTime
}

func (mp *ManagedProcess) setStartTime(startTime time.Time) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.StartTime = startTime
}

func (mp *ManagedProcess) getStopTime() time.Time {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.StopTime
}

func (mp *ManagedProcess) setStopTime(stopTime time.Time) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.StopTime = stopTime
}

func (mp *ManagedProcess) getNextRestartTime() time.Time {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.NextRestartTime
}

func (mp *ManagedProcess) setNextRestartTime(nextRestartTime time.Time) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.NextRestartTime = nextRestartTime
}

func (mp *ManagedProcess) getRestartCount() int {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	return mp.RestartCount
}

func (mp *ManagedProcess) setRestartCount(restartCount int) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.RestartCount = restartCount
}

func (mp *ManagedProcess) getDefaultLogFile(pm *ProgramManager, outFile string) string {
	const charset string = "abcdefghijklmnopqrstuvwxyz0123456789"
	const suffixLen int = 8

	num := ""
	if pm.Config.NumProcs > 1 {
		num = "_" + fmt.Sprintf("%02d", mp.Num)
	}

	suffix := make([]byte, suffixLen)
	for i := range suffixLen {
		suffix[i] = charset[rand.IntN(len(charset))]
	}

	logFileName := fmt.Sprintf("%s%s-%s---taskmaster-%s.log", pm.Name, num, outFile, suffix)

	return filepath.Join(pm.ChildLogDir, logFileName)
}

func (mp *ManagedProcess) newLogFile(pm *ProgramManager, path string, outFile string) (*os.File, string, error) {
	if strings.ToUpper(path) != "NONE" {
		if path == "" || strings.ToUpper(path) == "AUTO" {
			path = mp.getDefaultLogFile(pm, outFile)
		}

		logFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, "", fmt.Errorf("open %s failed: %w", outFile, err)
		}

		return logFile, path, nil
	}

	return nil, "", nil
}

func (mp *ManagedProcess) setProcessStdLogs(pm *ProgramManager) error {
	var err error

	if mp.StdoutLogFile == "" {
		mp.Stdout, mp.StdoutLogFile, err = mp.newLogFile(pm, pm.Config.StdoutLogFile, "stdout")
		if err != nil {
			return err
		}
	}

	if mp.StderrLogFile == "" {
		mp.Stderr, mp.StderrLogFile, err = mp.newLogFile(pm, pm.Config.StderrLogFile, "stderr")
		if err != nil {
			return err
		}
	}

	return nil
}

func (mp *ManagedProcess) newCmd(config *config.Program) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if config.Umask != nil {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("umask %03o; exec %s", *config.Umask, config.Cmd))
	} else {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("exec %s", config.Cmd))
	}

	cmd.Env = os.Environ()
	for envKey, envVal := range config.Env {
		cmd.Env = append(cmd.Env, envKey+"="+envVal)
	}

	cmd.Dir = config.WorkingDir

	user, err := user.Lookup(config.User)
	if err != nil {
		return nil, fmt.Errorf("user lookup failed: %w", err)
	}

	uid, _ := strconv.ParseInt(user.Uid, 10, 32)
	gid, _ := strconv.ParseInt(user.Gid, 10, 32)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
		Setpgid:    true,
		Pgid:       0,
	}

	cmd.Stdout = mp.Stdout
	cmd.Stderr = mp.Stderr

	return cmd, nil
}

func (mp *ManagedProcess) hasStartTimeoutExpired(startSecs int) bool {
	return time.Now().After(mp.getStartTime().Add(time.Duration(startSecs) * time.Second))
}

func (mp *ManagedProcess) hasStopTimeoutExpired(stopSecs int) bool {
	return time.Now().After(mp.getStopTime().Add(time.Duration(stopSecs) * time.Second))
}

func (mp *ManagedProcess) exitCodeExpected(processExitCode int, exitCodes []int) bool {
	for _, exitCode := range exitCodes {
		if processExitCode == exitCode {
			return true
		}
	}

	return false
}

func (mp *ManagedProcess) exitExpected(processExitCode int, exitCodes []int, startSecs int) bool {
	exitCodeExpected := mp.exitCodeExpected(processExitCode, exitCodes)
	uptime := mp.ExitTime.Sub(mp.getStartTime())

	return exitCodeExpected && uptime > time.Duration(startSecs)
}

func (mp *ManagedProcess) shouldRestart(autoRestart config.AutoRestart, exitCodes []int) bool {
	switch autoRestart {
	case config.AUTORESTART_NEVER:
		return false
	case config.AUTORESTART_ALWAYS:
		return true
	case config.AUTORESTART_ON_FAILURE:
		return !mp.exitCodeExpected(mp.getCmd().ProcessState.ExitCode(), exitCodes)
	default:
		return false
	}
}

func (mp *ManagedProcess) getDescription() string {
	state := mp.getState()
	switch state {
	case RUNNING:
		uptime := time.Since(mp.getStartTime())
		totalSeconds := int(uptime.Seconds())
		days := totalSeconds / 86400
		hours := (totalSeconds / 3600) % 24
		minutes := (totalSeconds / 60) % 60
		seconds := totalSeconds % 60

		if days > 0 {
			return fmt.Sprintf("pid %d, uptime %dd %02d:%02d:%02d", mp.Cmd.Process.Pid, days, hours, minutes, seconds)
		} else {
			return fmt.Sprintf("pid %d, uptime %02d:%02d:%02d", mp.Cmd.Process.Pid, hours, minutes, seconds)
		}
	case EXITED, STOPPED:
		if state == STOPPED && mp.ExitTime.Equal(time.Time{}) {
			return "not started"
		}

		return mp.ExitTime.Format("2006-01-02 15:04:05 MST")
	case BACKOFF, FATAL:
		return "exited too quickly"
	default:
		return ""
	}
}

func (mp *ManagedProcess) getStatus(processName string) *ProcessStatus {
	return &ProcessStatus{
		Name:        processName,
		State:       mp.getState(),
		Description: mp.getDescription(),
	}
}
