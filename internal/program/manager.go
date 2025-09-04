package program

import (
	"fmt"
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
	Action func(pm *ProgramManager, mp *ManagedProcess) Event
}

type ProgramManager struct {
	Name        string
	Config      *config.Program
	ChildLogDir string
	Log         *logger.Logger
	Processes   map[string]*ManagedProcess
	Transitions *map[State]map[Event]Transition
	StopChan    chan struct{}
	ExitChan    chan ProcessExitInfo
	Terminating bool
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	return &ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
		Processes:   make(map[string]*ManagedProcess),
		Transitions: newTransitions(),
		StopChan:    make(chan struct{}),
		ExitChan:    make(chan ProcessExitInfo),
		Terminating: false,
	}
}

func newTransitions() *map[State]map[Event]Transition {
	return &map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STOPPED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, RUNNING)
				mp.RestartCount = 0
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, STOPPING)
				return pm.stopProcess(mp)
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STARTING, BACKOFF)
				return pm.restartProcess(mp)
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, RUNNING, EXITED)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, RUNNING, STOPPING)
				return pm.stopProcess(mp)
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, BACKOFF, STARTING)
				return pm.startProcess(mp)
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, BACKOFF, FATAL)
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, STOPPING, STOPPED)
				if mp.RestartRequested {
					mp.RestartRequested = false
					return START
				}
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, EXITED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Num, FATAL, STARTING)
				return pm.startProcess(mp)
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

func (pm *ProgramManager) sendEvent(event Event, mp *ManagedProcess) {
	transition, found := (*pm.Transitions)[mp.State][event]
	if !found {
		pm.Log.Warningf("invalid event '%s' for state '%s'", event, mp.State)
		return
	}

	mp.State = transition.To
	if event := transition.Action(pm, mp); event != "" {
		pm.sendEvent(event, mp)
	}
}

func (pm *ProgramManager) startProcess(mp *ManagedProcess) Event {
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

	go func(mp *ManagedProcess) {
		exitInfo := ProcessExitInfo{Mp: mp, ExitTime: time.Now(), Err: mp.Cmd.Wait()}
		if exitInfo.Err != nil {
			pm.Log.Warningf("process %d exited: %v", mp.Cmd.Process.Pid, exitInfo.Err)
		}
		mp.ExitChan <- exitInfo
	}(mp)

	if pm.Config.StartSecs == 0 {
		return PROCESS_STARTED
	}

	return ""
}

func (pm *ProgramManager) restartProcess(mp *ManagedProcess) Event {
	if pm.Config.AutoRestart == config.AUTORESTART_NEVER || mp.RestartCount >= pm.Config.StartRetries {
		return TIMEOUT
	}

	mp.RestartCount++
	mp.NextRestartTime = time.Now().Add(time.Duration(mp.RestartCount) * time.Second)

	return ""
}

func (pm *ProgramManager) stopProcess(mp *ManagedProcess) Event {
	if mp.Cmd == nil || mp.Cmd.Process == nil {
		return PROCESS_STOPPED
	}

	pgid := mp.Cmd.Process.Pid
	if err := syscall.Kill(-pgid, pm.Config.StopSignal); err != nil {
		pm.Log.Warningf("failed to send stop signal to process %d: %v", mp.Cmd.Process.Pid, err)
		return PROCESS_STOPPED
	}

	mp.StopTime = time.Now()
	return ""
}

func (pm *ProgramManager) logTransition(processNum int, from State, to State) {
	pm.Log.Infof("%s %s -> %s\n", pm.getProcessName(processNum), from, to)
}

func (pm *ProgramManager) forceStop(mp *ManagedProcess) {
	if mp.Cmd == nil || mp.Cmd.Process == nil {
		pm.sendEvent(PROCESS_STOPPED, mp)
	}

	pm.Log.Warningf("process %d did not stop gracefully, sending SIGKILL", mp.Cmd.Process.Pid)
	pgid := mp.Cmd.Process.Pid
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		pm.Log.Errorf("failed to SIGKILL process %d: %v", mp.Cmd.Process.Pid, err)
	}
}

func (pm *ProgramManager) initiateShutdown() {
	if !pm.Terminating {
		pm.Terminating = true
		for _, mp := range pm.Processes {
			if mp.State == RUNNING || mp.State == STARTING {
				pm.sendEvent(STOP, mp)
			}
		}
	}
}

func (pm *ProgramManager) allProcessesTerminated() bool {
	for _, mp := range pm.Processes {
		if mp.State != STOPPED && mp.State != EXITED && mp.State != BACKOFF && mp.State != FATAL {
			return false
		}
	}

	return true
}

func (pm *ProgramManager) StartAllProcesses() {
	for processName, mp := range pm.Processes {
		switch mp.State {
		case RUNNING, STARTING:
			pm.Log.Infof("%s already %s", processName, mp.State)
			continue
		default:
			pm.sendEvent(START, mp)
		}
	}
}

func (pm *ProgramManager) StopAllProcesses() {
	for processName, mp := range pm.Processes {
		switch mp.State {
		case STOPPED, EXITED, FATAL, BACKOFF:
			pm.Log.Infof("%s already %s", processName, mp.State)
			continue
		default:
			pm.sendEvent(STOP, mp)
		}
	}
}

func (pm *ProgramManager) RestartAllProcesses() {
	for _, mp := range pm.Processes {
		switch mp.State {
		case RUNNING, STARTING:
			mp.RestartRequested = true
			pm.sendEvent(STOP, mp)
		case STOPPED, EXITED, FATAL, BACKOFF:
			pm.sendEvent(START, mp)
		}
	}
}

func (pm *ProgramManager) Run() {
	pm.Log.Debugf("%s: %+v\n\n", pm.Name, pm.Config)

	for processNum := range pm.Config.NumProcs {
		processName := pm.getProcessName(processNum)
		pm.Processes[processName] = newManagedProcess(processNum, pm.ExitChan)

		if pm.Config.AutoStart {
			pm.sendEvent(START, pm.Processes[processName])
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pm.StopChan:
			pm.initiateShutdown()

			if pm.allProcessesTerminated() {
				pm.Log.Infof("stopped program manager: %s", pm.Name)
				return
			}
		case exit := <-pm.ExitChan:
			mp := exit.Mp
			mp.ExitTime = exit.ExitTime

			if mp.State == STOPPING {
				pm.sendEvent(PROCESS_STOPPED, mp)
			} else {
				pm.sendEvent(PROCESS_EXITED, mp)
			}
		case <-ticker.C:
			for _, mp := range pm.Processes {
				if mp.State == STARTING && mp.hasStartTimeoutExpired(pm.Config.StartSecs) {
					pm.sendEvent(PROCESS_STARTED, mp)
				} else if mp.State == STOPPING && mp.hasStopTimeoutExpired(pm.Config.StopSecs) {
					pm.forceStop(mp)
				} else if mp.State == EXITED && mp.shouldRestart(pm.Config.AutoRestart, pm.Config.ExitCodes) && !pm.Terminating {
					pm.sendEvent(START, mp)
				} else if mp.State == BACKOFF && time.Now().After(mp.NextRestartTime) && !pm.Terminating {
					pm.sendEvent(START, mp)
				}
			}
		}
	}
}

func (pm *ProgramManager) Stop() {
	pm.Log.Infof("stopping program manager: %s", pm.Name)
	close(pm.StopChan)
}
