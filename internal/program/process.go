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
	"syscall"
	"time"

	"taskmaster/internal/config"
)

type ProcessExitInfo struct {
	Mp       *ManagedProcess
	ExitTime time.Time
	Err      error
}

type RequestReply struct {
	ProcessName string
	Message     string
	Err         error
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
	RestartCount    int
	ExitChan        chan ProcessExitInfo
	Stdout          *os.File
	Stderr          *os.File
	StdoutLogFile   string
	StderrLogFile   string
}

func newManagedProcess(processNum int, processName string, exitChan chan ProcessExitInfo) *ManagedProcess {
	return &ManagedProcess{
		Name:     processName,
		Num:      processNum,
		State:    STOPPED,
		ExitChan: exitChan,
	}
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
	return time.Now().After(mp.StartTime.Add(time.Duration(startSecs) * time.Second))
}

func (mp *ManagedProcess) hasStopTimeoutExpired(stopSecs int) bool {
	return time.Now().After(mp.StopTime.Add(time.Duration(stopSecs) * time.Second))
}

func (mp *ManagedProcess) shouldRestart(autoRestart config.AutoRestart, exitCodes []int) bool {
	switch autoRestart {
	case config.AUTORESTART_NEVER:
		return false
	case config.AUTORESTART_ALWAYS:
		return true
	case config.AUTORESTART_ON_FAILURE:
		for _, exitCode := range exitCodes {
			if mp.Cmd.ProcessState.ExitCode() == exitCode {
				return false
			}
		}

		return true
	default:
		return false
	}
}

func (mp *ManagedProcess) getDescription() string {
	switch mp.State {
	case RUNNING:
		uptime := time.Since(mp.StartTime)
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
		State:       mp.State,
		Description: mp.getDescription(),
	}
}
