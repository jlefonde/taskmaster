package program

import (
	"fmt"
	"os/exec"
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

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
	Processes   map[string]*ManagedProcess
	Transitions *map[State]map[Event]Transition
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

func newTransitions() *map[State]map[Event]Transition {
	return &map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STOPPED, STARTING)
				pm.printTransition(mp.Num, STOPPED, STARTING)
				return pm.startCmd(mp)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, RUNNING)
				pm.printTransition(mp.Num, STARTING, RUNNING)
				mp.RestartCount = 0
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, STOPPING)
				pm.printTransition(mp.Num, STARTING, STOPPING)
				return ""
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, BACKOFF)
				pm.printTransition(mp.Num, STARTING, BACKOFF)
				return pm.restartCmd(mp)
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, RUNNING, EXITED)
				pm.printTransition(mp.Num, RUNNING, EXITED)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, RUNNING, STOPPING)
				pm.printTransition(mp.Num, RUNNING, STOPPING)
				return ""
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, BACKOFF, STARTING)
				pm.printTransition(mp.Num, BACKOFF, STARTING)
				return pm.startCmd(mp)
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, BACKOFF, FATAL)
				pm.printTransition(mp.Num, BACKOFF, FATAL)
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STOPPING, STOPPED)
				pm.printTransition(mp.Num, STOPPING, STOPPED)
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, EXITED, STARTING)
				pm.printTransition(mp.Num, EXITED, STARTING)
				return pm.startCmd(mp)
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, FATAL, STARTING)
				pm.printTransition(mp.Num, FATAL, STARTING)
				return ""
			}},
		},
	}
}

func (pm *ProgramManager) getProcessName(processNum int) string {
	if pm.Config.NumProcs == 1 {
		return pm.Name
	}

	return fmt.Sprintf("%s:%s_%02d", pm.Name, pm.Name, processNum)
}

func (pm *ProgramManager) sendEvent(event Event, mp *ManagedProcess) error {
	transition, found := (*pm.Transitions)[mp.State][event]
	if !found {
		return fmt.Errorf("invalid event '%q' for state '%q'", event, mp.State)
	}

	mp.State = transition.To
	if event := transition.Action(pm, mp); event != "" {
		return pm.sendEvent(event, mp)
	}

	return nil
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

	if pm.Config.StartSecs == 0 {
		return PROCESS_STARTED
	}

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

func (pm *ProgramManager) logTransition(processNum int, from State, to State) {
	pm.Log.Infof("%s %s -> %s\n", pm.getProcessName(processNum), from, to)
}

func (pm *ProgramManager) printTransition(processNum int, from State, to State) {
	fmt.Printf("%s %s -> %s\n", pm.getProcessName(processNum), from, to)
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
				} else if mp.State == EXITED && mp.shouldRestart(pm.Config.AutoRestart, pm.Config.ExitCodes) {
					pm.sendEvent(START, mp)
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}
