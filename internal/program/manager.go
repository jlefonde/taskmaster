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

type State string
type Event string

const (
	STOPPED  State = "STOPPED"
	STARTING State = "STARTING"
	RUNNING  State = "RUNNING"
	BACKOFF  State = "BACKOFF"
	STOPPING State = "STOPPING"
	EXITED   State = "EXITED"
	FATAL    State = "FATAL"

	START           Event = "START"
	STOP            Event = "STOP"
	PROCESS_STARTED Event = "PROCESS_STARTED"
	PROCESS_STOPPED Event = "PROCESS_STOPPED"
	PROCESS_EXITED  Event = "PROCESS_EXITED"
	PROCESS_FAILED  Event = "PROCESS_FAILED"
	TIMEOUT         Event = "TIMEOUT"
)

type ProcessExitInfo struct {
	ExitTime time.Time
	Err      error
}

type Transition struct {
	To     State
	Action func(pm *ProgramManager, mp *ManagedProcess) Event
}

type ManagedProcess struct {
	Num             int
	Cmd             *exec.Cmd
	State           State
	StartTime       time.Time
	ExitTime        time.Time
	NextRestartTime time.Time
	RestartCount    int
	ExitChan        chan ProcessExitInfo
	Stdout          *os.File
	Stderr          *os.File
	StdoutLogFile   string
	StderrLogFile   string
}

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
	Processes   map[string]*ManagedProcess
	Transitions *map[State]map[Event]Transition
}

func (pm *ProgramManager) startCmd(mp *ManagedProcess) Event {
	if err := mp.setProcessStdLogs(pm); err != nil {
		pm.Log.Warningf("failed to set logs file for program '%s' (process %d): %v", pm.Name, mp.Num, err)
		return PROCESS_EXITED
	}

	cmd, err := mp.newCmd(pm.Config)
	if err != nil {
		pm.Log.Warningf("prepare cmd failed for program '%s' (process %d): %v", pm.Name, mp.Num, err)
		return PROCESS_EXITED
	}

	if err := cmd.Start(); err != nil {
		return PROCESS_EXITED
	}

	mp.StartTime = time.Now()
	mp.Cmd = cmd

	go func(cmd *exec.Cmd) {
		mp.ExitChan <- ProcessExitInfo{ExitTime: time.Now(), Err: cmd.Wait()}
	}(cmd)

	return ""
}

func (pm *ProgramManager) restartCmd(mp *ManagedProcess) Event {
	if pm.Config.AutoRestart == config.AUTORESTART_NEVER || mp.RestartCount >= pm.Config.StartRetries {
		return TIMEOUT
	}

	mp.RestartCount++

	mp.NextRestartTime = time.Now().Add(time.Duration(mp.RestartCount) * time.Second)

	return ""
}

func newTransitions() *map[State]map[Event]Transition {
	return &map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("STOPPED -> STARTING")
				return pm.startCmd(mp)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("STARTING -> RUNNING")
				mp.RestartCount = 0
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("STARTING -> STOPPING")
				return ""
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("STARTING -> BACKOFF")
				return pm.restartCmd(mp)
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("RUNNING -> EXITED")
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("RUNNING -> STOPPING")
				return ""
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("BACKOFF -> STARTING")
				return pm.startCmd(mp)
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("BACKOFF -> FATAL")
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("STOPPING -> STOPPED")
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("EXITED -> STARTING")
				return pm.startCmd(mp)
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				fmt.Println("FATAL -> STARTING")
				return ""
			}},
		},
	}
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	return &ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
		Processes:   make(map[string]*ManagedProcess),
		Transitions: newTransitions(),
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
	if s.ToUpper(path) != "NONE" {
		if path == "" || s.ToUpper(path) == "AUTO" {
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

func (pm *ProgramManager) getProcessName(processNum int) string {
	if pm.Config.NumProcs == 1 {
		return pm.Name
	}

	return fmt.Sprintf("%s:%s_%02d", pm.Name, pm.Name, processNum)
}

func (mp *ManagedProcess) newCmd(config *config.Program) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if config.Umask != nil {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("umask %03o; exec %s", *config.Umask, config.Cmd))
	} else {
		cmd = exec.Command("sh", "-c", config.Cmd)
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
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}

	cmd.Stdout = mp.Stdout
	cmd.Stderr = mp.Stderr

	return cmd, nil
}

func (pm *ProgramManager) sendEvent(event Event, mp *ManagedProcess) error {
	transition, found := (*pm.Transitions)[mp.State][event]
	if !found {
		return fmt.Errorf("invalid event '%q' for state '%q'", event, mp.State)
	}

	// fmt.Printf("process_%02d: %+v\n", mp.Num, pm.Processes[processName])

	mp.State = transition.To
	if event := transition.Action(pm, mp); event != "" {
		return pm.sendEvent(event, mp)
	}

	return nil
}

func (mp *ManagedProcess) isAutoRestart(autoRestart config.AutoRestart, exitCodes []int) bool {
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

func (pm *ProgramManager) Run() error {
	fmt.Printf("%s: %+v\n\n", pm.Name, pm.Config)

	for processNum := range pm.Config.NumProcs {
		processName := pm.getProcessName(processNum)
		pm.Processes[processName] = &ManagedProcess{
			Num:      processNum,
			State:    STOPPED,
			ExitChan: make(chan ProcessExitInfo, 1),
		}

		if pm.Config.AutoStart {
			pm.sendEvent(START, pm.Processes[processName])
		}
	}

	for {
		for _, mp := range pm.Processes {
			select {
			case exitInfo := <-mp.ExitChan:
				mp.ExitTime = exitInfo.ExitTime
				pm.sendEvent(PROCESS_EXITED, mp)
			default:
				if mp.State == STARTING && time.Now().After(mp.StartTime.Add(time.Duration(pm.Config.StartSecs)*time.Second)) {
					pm.sendEvent(PROCESS_STARTED, mp)
				} else if mp.State == BACKOFF && time.Now().After(mp.NextRestartTime) {
					pm.sendEvent(START, mp)
				} else if mp.State == EXITED && mp.isAutoRestart(pm.Config.AutoRestart, pm.Config.ExitCodes) {
					pm.sendEvent(START, mp)
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}
