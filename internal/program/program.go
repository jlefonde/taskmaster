package program

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
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
	Requests    map[string]chan<- RequestReply
	Transitions *map[State]map[Event]Transition
	StopChan    chan struct{}
	ExitChan    chan ProcessExitInfo
	doneChan    chan struct{}
	Terminating bool
	mu          sync.Mutex
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	return &ProgramManager{
		Name:        programName,
		Config:      programConfig,
		ChildLogDir: childLogDir,
		Log:         log,
		Processes:   make(map[string]*ManagedProcess),
		Requests:    make(map[string]chan<- RequestReply),
		Transitions: newTransitions(),
		StopChan:    make(chan struct{}),
		ExitChan:    make(chan ProcessExitInfo),
		doneChan:    make(chan struct{}),
		Terminating: false,
	}
}

func newTransitions() *map[State]map[Event]Transition {
	return &map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STOPPED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STARTING, RUNNING)
				pm.replyToRequest(mp, "started", false)
				pm.Log.Infof("success: '%s' entered RUNNING state, process has stayed up for > than %d seconds (startsecs)",
					mp.Name, pm.Config.StartSecs)
				mp.setRestartCount(0)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STARTING, STOPPING)
				return pm.stopProcess(mp)
			}},
			PROCESS_FAILED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STARTING, BACKOFF)
				pm.replyToRequest(mp, "spawn error", true)
				return pm.restartProcess(mp)
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STARTING, BACKOFF)
				pm.replyToRequest(mp, "spawn error", true)
				return pm.restartProcess(mp)
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, RUNNING, EXITED)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, RUNNING, STOPPING)
				return pm.stopProcess(mp)
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, BACKOFF, STARTING)
				return pm.startProcess(mp)
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, BACKOFF, FATAL)
				pm.Log.Infof("gave up: '%s' entered FATAL state, too many start retries too quickly", mp.Name)
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, STOPPING, STOPPED)
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, EXITED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.Name, FATAL, STARTING)
				return pm.startProcess(mp)
			}},
		},
	}
}

func (pm *ProgramManager) getProcessNames() []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	names := make([]string, 0, len(pm.Processes))
	for processName := range pm.Processes {
		names = append(names, processName)
	}
	return names
}

func (pm *ProgramManager) getRequest(processName string) (chan<- RequestReply, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, ok := pm.Requests[processName]

	return req, ok
}

func (pm *ProgramManager) setRequest(processName string, replyChan chan<- RequestReply) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.Requests[processName] = replyChan
}

func (pm *ProgramManager) deleteRequest(processName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.Requests, processName)
}

func (pm *ProgramManager) getProcessName(processNum int) string {
	if pm.Config.NumProcs == 1 {
		return pm.Name
	}

	return fmt.Sprintf("%s:%s_%02d", pm.Name, pm.Name, processNum)
}

func (pm *ProgramManager) sendEvent(event Event, mp *ManagedProcess) {
	state := mp.getState()
	transition, found := (*pm.Transitions)[state][event]
	if !found {
		pm.Log.Warningf("invalid event '%s' for state '%s'", event, state)
		return
	}

	mp.setState(transition.To)
	if event := transition.Action(pm, mp); event != "" {
		pm.sendEvent(event, mp)
	}
}

func (pm *ProgramManager) replyToRequest(mp *ManagedProcess, msg string, isError bool) {
	replyChan, ok := pm.getRequest(mp.Name)
	if ok {
		if isError {
			replyChan <- RequestReply{
				Name: mp.Name,
				Err:  fmt.Errorf("%s", msg),
			}
		} else {
			replyChan <- RequestReply{
				Name:    mp.Name,
				Message: msg,
			}
		}

		pm.deleteRequest(mp.Name)
	}
}

func (pm *ProgramManager) startProcess(mp *ManagedProcess) Event {
	mp.setStartTime(time.Time{})

	if err := mp.setProcessStdLogs(pm); err != nil {
		pm.Log.Warningf("failed to set logs file for %s: %v", mp.Name, err)
		return PROCESS_FAILED
	}

	cmd, err := mp.newCmd(pm.Config)
	if err != nil {
		pm.Log.Warningf("failed to create command for %s: %v", mp.Name, err)
		return PROCESS_FAILED
	}

	if err := cmd.Start(); err != nil {
		return PROCESS_FAILED
	}

	mp.setStartTime(time.Now())
	mp.setCmd(cmd)

	pm.Log.Infof("spawned: '%s' with pid %d", mp.Name, mp.getCmd().Process.Pid)

	go func(mp *ManagedProcess) {
		exitInfo := ProcessExitInfo{Mp: mp, ExitTime: time.Now(), Err: mp.getCmd().Wait()}
		mp.ExitChan <- exitInfo
	}(mp)

	if pm.Config.StartSecs == 0 {
		return PROCESS_STARTED
	}

	return ""
}

func (pm *ProgramManager) restartProcess(mp *ManagedProcess) Event {
	restartCount := mp.getRestartCount()
	if restartCount >= pm.Config.StartRetries {
		return TIMEOUT
	}

	mp.setRestartCount(restartCount + 1)
	mp.setNextRestartTime(time.Now().Add(time.Duration(restartCount+1) * time.Second))

	return ""
}

func (pm *ProgramManager) stopProcess(mp *ManagedProcess) Event {
	pgid := mp.getCmd().Process.Pid
	pm.Log.Warningf("killing '%s' (%d) with %s", mp.Name, pgid, pm.Config.StopSignal)
	if err := syscall.Kill(-pgid, pm.Config.StopSignal); err != nil {
		pm.Log.Warningf("failed to send stop signal to '%s' (%d): %v", mp.Name, pgid, err)
		pm.replyToRequest(mp, "failed to stop", true)
		return PROCESS_STOPPED
	}

	mp.setStopTime(time.Now())

	return ""
}

func (pm *ProgramManager) logTransition(processName string, from State, to State) {
	pm.Log.Debugf("%s [%s -> %s]", processName, from, to)
}

func (pm *ProgramManager) forceStop(mp *ManagedProcess) {
	mp.StoppedSignal = syscall.SIGKILL

	pgid := mp.getCmd().Process.Pid
	pm.Log.Warningf("killing '%s' (%d) with sigkill: process didn't stop gracefully", mp.Name, pgid)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		pm.Log.Errorf("failed to send sigkill to '%s' (%d): %v", mp.Name, pgid, err)
	}
}

func (pm *ProgramManager) initiateShutdown() {
	if !pm.Terminating {
		pm.Terminating = true
		for _, mp := range pm.Processes {
			state := mp.getState()
			if state == RUNNING || state == STARTING {
				pm.sendEvent(STOP, mp)
			}
		}
	}
}

func (pm *ProgramManager) allProcessesTerminated() bool {
	for _, mp := range pm.Processes {
		state := mp.getState()
		if state != STOPPED && state != EXITED && state != BACKOFF && state != FATAL {
			return false
		}
	}

	return true
}

func (pm *ProgramManager) StartProcess(processName string, replyChan chan<- RequestReply) {
	mp, ok := pm.Processes[processName]
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.getState()
	switch state {
	case RUNNING, STARTING:
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("already %s", strings.ToLower(string(state))),
		}
	default:
		pm.setRequest(processName, replyChan)
		pm.sendEvent(START, mp)
	}
}

func (pm *ProgramManager) StartAllProcesses(replyChan chan<- []RequestReply) {
	processNames := pm.getProcessNames()
	processCount := len(processNames)
	processReplyChan := make(chan RequestReply, processCount)

	for _, processName := range processNames {
		go func(processName string) {
			pm.StartProcess(processName, processReplyChan)
		}(processName)
	}

	var replies []RequestReply
	for range processCount {
		replies = append(replies, <-processReplyChan)
	}

	replyChan <- replies
}

func (pm *ProgramManager) StopProcess(processName string, replyChan chan<- RequestReply) {
	mp, ok := pm.Processes[processName]
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.getState()
	switch state {
	case STOPPED, EXITED, FATAL, BACKOFF:
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("already %s", strings.ToLower(string(state))),
		}
	default:
		pm.setRequest(processName, replyChan)
		pm.sendEvent(STOP, mp)
	}
}

func (pm *ProgramManager) StopAllProcesses(replyChan chan<- []RequestReply) {
	processNames := pm.getProcessNames()
	processCount := len(processNames)
	processReplyChan := make(chan RequestReply, processCount)

	for _, processName := range processNames {
		go func(processName string) {
			pm.StopProcess(processName, processReplyChan)
		}(processName)
	}

	var replies []RequestReply
	for range processCount {
		replies = append(replies, <-processReplyChan)
	}

	replyChan <- replies
}

func (pm *ProgramManager) GetProcessPID(processName string, replyChan chan<- RequestReply) {
	mp, ok := pm.Processes[processName]
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.getState()
	if state == RUNNING {
		replyChan <- RequestReply{
			Name:    processName,
			Message: strconv.Itoa(mp.getCmd().Process.Pid),
		}
	} else {
		replyChan <- RequestReply{
			Name: processName,
			Err:  errors.New("not running"),
		}
	}
}

func (pm *ProgramManager) GetAllProcessPIDs(replyChan chan<- []RequestReply) {
	processCount := len(pm.Processes)
	processReplyChan := make(chan RequestReply, processCount)

	for processName := range pm.Processes {
		go func(processName string) {
			pm.GetProcessPID(processName, processReplyChan)
		}(processName)
	}

	var replies []RequestReply
	for range processCount {
		replies = append(replies, <-processReplyChan)
	}

	replyChan <- replies
}

func (pm *ProgramManager) GetAllProcessesStatus(replyChan chan<- []ProcessStatus) {
	processCount := len(pm.Processes)
	processReplyChan := make(chan ProcessStatus, processCount)

	for processName := range pm.Processes {
		go func(processName string) {
			pm.GetProcessStatus(processName, processReplyChan)
		}(processName)
	}

	var replies []ProcessStatus
	for range processCount {
		replies = append(replies, <-processReplyChan)
	}

	replyChan <- replies
}

func (pm *ProgramManager) GetProcessStatus(processName string, replyChan chan<- ProcessStatus) {
	mp, ok := pm.Processes[processName]
	if !ok {
		replyChan <- ProcessStatus{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	replyChan <- *mp.getStatus(processName)
}

func (pm *ProgramManager) checkProcessState(mp *ManagedProcess) {
	state := mp.getState()
	if state == STARTING && mp.hasStartTimeoutExpired(pm.Config.StartSecs) {
		pm.sendEvent(PROCESS_STARTED, mp)
	} else if state == STOPPING {
		if mp.hasStopTimeoutExpired(pm.Config.StopSecs) {
			pm.forceStop(mp)
		} else {
			pm.Log.Infof("waiting for '%s' to stop", mp.Name)
		}
	} else if state == EXITED && mp.shouldRestart(pm.Config.AutoRestart, pm.Config.ExitCodes) && !pm.Terminating {
		pm.sendEvent(START, mp)
	} else if state == BACKOFF && time.Now().After(mp.getNextRestartTime()) && !pm.Terminating {
		pm.sendEvent(START, mp)
	}
}

func (pm *ProgramManager) Run() {
	defer close(pm.doneChan)

	for processNum := range pm.Config.NumProcs {
		processName := pm.getProcessName(processNum)
		pm.Processes[processName] = newManagedProcess(processNum, processName, pm.ExitChan, pm.Config.StopSignal)

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
				return
			}
		case exit := <-pm.ExitChan:
			mp := exit.Mp
			mp.ExitTime = exit.ExitTime

			state := mp.getState()
			if state == STOPPING {
				pm.sendEvent(PROCESS_STOPPED, mp)

				pm.Log.Infof("stopped: %s (%s)", mp.Name, mp.StoppedSignal)
				pm.replyToRequest(mp, "stopped", false)
			} else {
				pm.sendEvent(PROCESS_EXITED, mp)

				exitCode := mp.getCmd().ProcessState.ExitCode()
				if mp.exitExpected(exitCode, pm.Config.ExitCodes, pm.Config.StartSecs) {
					pm.Log.Infof("exited: %s (exit status %d; expected)", mp.Name, exitCode)
				} else {
					pm.Log.Warningf("exited: %s (exit status %d; not expected)", mp.Name, exitCode)
				}

				pm.replyToRequest(mp, fmt.Sprintf("already %s", strings.ToLower(string(state))), true)
			}
		case <-ticker.C:
			for _, mp := range pm.Processes {
				go pm.checkProcessState(mp)
			}
		}
	}
}

func (pm *ProgramManager) Stop() {
	close(pm.StopChan)
}

func (pm *ProgramManager) Wait() {
	<-pm.doneChan
}
