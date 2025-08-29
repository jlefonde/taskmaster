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

type Transition struct {
	To     State
	Action func(pm *ProgramManager, processName string) Event
}

type ManagedProcess struct {
	Num      int
	Process  *os.Process
	State    State
	ExitChan chan error
}

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
	Processes   map[string]*ManagedProcess
	Transitions *map[State]map[Event]Transition
}

func (pm *ProgramManager) startCmd(processName string) Event {
	mp := pm.Processes[processName]

	cmd, err := pm.newCmd(mp.Num)
	if err != nil {
		pm.Log.Warningf("prepare cmd failed for program '%s' (process %d): %v", pm.Name, mp.Num, err)
		return PROCESS_EXITED
	}

	if err := cmd.Start(); err != nil {
		return PROCESS_EXITED
	}

	mp.Process = cmd.Process

	go func(cmd *exec.Cmd) {
		mp.ExitChan <- cmd.Wait()
	}(cmd)

	return PROCESS_STARTED
}

func newTransitions() *map[State]map[Event]Transition {
	transitionGraph := map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("STOPPED -> STARTING")
				return pm.startCmd(processName)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("STARTING -> RUNNING")
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("STARTING -> STOPPING")
				return ""
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("STARTING -> BACKOFF")
				return ""
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("RUNNING -> EXITED")
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("RUNNING -> STOPPING")
				return ""
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("BACKOFF -> STARTING")
				return ""
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("BACKOFF -> FATAL")
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("STOPPING -> STOPPED")
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("EXITED -> STARTING")
				return ""
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, processName string) Event {
				fmt.Println("FATAL -> STARTING")
				return ""
			}},
		},
	}

	return &transitionGraph
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	pm := ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
		Processes:   make(map[string]*ManagedProcess),
		Transitions: newTransitions(),
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

func (pm *ProgramManager) newCmd(processNum int) (*exec.Cmd, error) {
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

func (pm *ProgramManager) sendEvent(event Event, processName string) error {
	mp, found := pm.Processes[processName]
	if !found {
		return fmt.Errorf("process '%s' not found", processName)
	}

	transition, found := (*pm.Transitions)[mp.State][event]
	if !found {
		return fmt.Errorf("invalid event '%q' for state '%q'", event, mp.State)
	}

	fmt.Printf("process_%02d: %+v\n", mp.Num, pm.Processes[processName])

	mp.State = transition.To
	if event := transition.Action(pm, processName); event != "" {
		return pm.sendEvent(event, processName)
	}

	return nil
}

func (pm *ProgramManager) Run() error {
	fmt.Printf("%s: %+v\n\n", pm.Name, pm.Config)

	for processNum := range pm.Config.NumProcs {
		processName := pm.getProcessName(processNum)
		pm.Processes[processName] = &ManagedProcess{
			Num:      processNum,
			State:    STOPPED,
			ExitChan: make(chan error, 1),
		}

		if pm.Config.AutoStart {
			pm.sendEvent(START, processName)
		}

		fmt.Printf("process_%02d: %+v\n", processNum, pm.Processes[processName])
	}

	for {
		select {}
		time.Sleep(10 * time.Second)
	}

	// fmt.Println("")

	return nil
}
