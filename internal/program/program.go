package program

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jlefonde/taskmaster/internal/config"
	"github.com/jlefonde/taskmaster/internal/logger"
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
	name        string
	Config      *config.Program
	childLogDir string
	log         *logger.Logger
	processes   map[string]*ManagedProcess
	requests    map[string]chan<- RequestReply
	transitions *map[State]map[Event]Transition
	stopChan    chan struct{}
	exitChan    chan ProcessExitInfo
	doneChan    chan struct{}
	terminating bool
	mu          sync.Mutex
}

func NewProgramManager(programName string, programConfig *config.Program, childLogDir string, log *logger.Logger) *ProgramManager {
	return &ProgramManager{
		name:        programName,
		Config:      programConfig,
		childLogDir: childLogDir,
		log:         log,
		processes:   make(map[string]*ManagedProcess),
		requests:    make(map[string]chan<- RequestReply),
		transitions: newTransitions(),
		stopChan:    make(chan struct{}),
		exitChan:    make(chan ProcessExitInfo),
		doneChan:    make(chan struct{}),
		terminating: false,
	}
}

func newTransitions() *map[State]map[Event]Transition {
	return &map[State]map[Event]Transition{
		STOPPED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STOPPED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		STARTING: {
			PROCESS_STARTED: {To: RUNNING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STARTING, RUNNING)
				pm.replyToRequest(mp, "started", false)
				pm.log.Infof("success: '%s' entered RUNNING state, process has stayed up for > than %d seconds (startsecs)",
					mp.name, pm.Config.StartSecs)
				mp.SetRestartCount(0)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STARTING, STOPPING)
				return pm.stopProcess(mp)
			}},
			PROCESS_FAILED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STARTING, BACKOFF)
				pm.replyToRequest(mp, "spawn error", true)
				return pm.restartProcess(mp)
			}},
			PROCESS_EXITED: {To: BACKOFF, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STARTING, BACKOFF)
				pm.replyToRequest(mp, "spawn error", true)
				return pm.restartProcess(mp)
			}},
		},
		RUNNING: {
			PROCESS_EXITED: {To: EXITED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, RUNNING, EXITED)
				return ""
			}},
			STOP: {To: STOPPING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, RUNNING, STOPPING)
				return pm.stopProcess(mp)
			}},
		},
		BACKOFF: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, BACKOFF, STARTING)
				return pm.startProcess(mp)
			}},
			TIMEOUT: {To: FATAL, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, BACKOFF, FATAL)
				pm.log.Infof("gave up: '%s' entered FATAL state, too many start retries too quickly", mp.name)
				return ""
			}},
		},
		STOPPING: {
			PROCESS_STOPPED: {To: STOPPED, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, STOPPING, STOPPED)
				return ""
			}},
		},
		EXITED: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, EXITED, STARTING)
				return pm.startProcess(mp)
			}},
		},
		FATAL: {
			START: {To: STARTING, Action: func(pm *ProgramManager, mp *ManagedProcess) Event {
				pm.logTransition(mp.name, FATAL, STARTING)
				return pm.startProcess(mp)
			}},
		},
	}
}

func (pm *ProgramManager) Name() string {
	return pm.name
}

func (pm *ProgramManager) Processes() map[string]*ManagedProcess {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.processes
}

func (pm *ProgramManager) GetProcess(processName string) (*ManagedProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	mp, ok := pm.processes[processName]
	return mp, ok
}

func (pm *ProgramManager) SetProcess(processName string, mp *ManagedProcess) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.processes[processName] = mp
}

func (pm *ProgramManager) GetRequest(processName string) (chan<- RequestReply, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, ok := pm.requests[processName]

	return req, ok
}

func (pm *ProgramManager) getProcessNames() []string {
	processes := pm.Processes()
	names := make([]string, 0, len(processes))
	for processName := range processes {
		names = append(names, processName)
	}

	return names
}

func (pm *ProgramManager) SetRequest(processName string, replyChan chan<- RequestReply) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.requests[processName] = replyChan
}

func (pm *ProgramManager) DeleteRequest(processName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.requests, processName)
}

func (pm *ProgramManager) getProcessName(processNum int) string {
	if pm.Config.NumProcs == 1 {
		return pm.name
	}

	return fmt.Sprintf("%s:%s_%02d", pm.name, pm.name, processNum)
}

func (pm *ProgramManager) sendEvent(event Event, mp *ManagedProcess) {
	state := mp.State()
	transition, found := (*pm.transitions)[state][event]
	if !found {
		pm.log.Warningf("invalid event '%s' for state '%s'", event, state)
		return
	}

	mp.SetState(transition.To)
	if event := transition.Action(pm, mp); event != "" {
		pm.sendEvent(event, mp)
	}
}

func (pm *ProgramManager) replyToRequest(mp *ManagedProcess, msg string, isError bool) {
	replyChan, ok := pm.GetRequest(mp.name)
	if ok {
		if isError {
			replyChan <- RequestReply{
				Name: mp.name,
				Err:  fmt.Errorf("%s", msg),
			}
		} else {
			replyChan <- RequestReply{
				Name:    mp.name,
				Message: msg,
			}
		}

		pm.DeleteRequest(mp.name)
	}
}

func (pm *ProgramManager) startProcess(mp *ManagedProcess) Event {
	mp.SetStartTime(time.Time{})

	if err := mp.setProcessStdLogs(pm); err != nil {
		pm.log.Warningf("failed to set logs file for %s: %v", mp.name, err)
		return PROCESS_FAILED
	}

	cmd, err := mp.newCmd(pm.Config)
	if err != nil {
		pm.log.Warningf("failed to create command for %s: %v", mp.name, err)
		return PROCESS_FAILED
	}

	if err := cmd.Start(); err != nil {
		return PROCESS_FAILED
	}

	mp.SetStartTime(time.Now())
	mp.SetCmd(cmd)

	pm.log.Infof("spawned: '%s' with pid %d", mp.name, mp.Cmd().Process.Pid)

	go func(mp *ManagedProcess) {
		exitInfo := ProcessExitInfo{Mp: mp, ExitTime: time.Now(), Err: mp.Cmd().Wait()}
		mp.exitChan <- exitInfo
	}(mp)

	if pm.Config.StartSecs == 0 {
		return PROCESS_STARTED
	}

	return ""
}

func (pm *ProgramManager) restartProcess(mp *ManagedProcess) Event {
	restartCount := mp.RestartCount()
	if restartCount >= pm.Config.StartRetries {
		return TIMEOUT
	}

	mp.SetRestartCount(restartCount + 1)
	mp.SetNextRestartTime(time.Now().Add(time.Duration(restartCount+1) * time.Second))

	return ""
}

func (pm *ProgramManager) stopProcess(mp *ManagedProcess) Event {
	pgid := mp.Cmd().Process.Pid
	pm.log.Warningf("killing '%s' (%d) with %s", mp.name, pgid, pm.Config.StopSignal)
	if err := syscall.Kill(-pgid, pm.Config.StopSignal); err != nil {
		pm.log.Warningf("failed to send stop signal to '%s' (%d): %v", mp.name, pgid, err)
		pm.replyToRequest(mp, "failed to stop", true)
		return PROCESS_STOPPED
	}

	mp.SetStopTime(time.Now())

	return ""
}

func (pm *ProgramManager) logTransition(processName string, from State, to State) {
	pm.log.Debugf("%s [%s -> %s]", processName, from, to)
}

func (pm *ProgramManager) forceStop(mp *ManagedProcess) {
	mp.stoppedSignal = syscall.SIGKILL

	pgid := mp.Cmd().Process.Pid
	pm.log.Warningf("killing '%s' (%d) with sigkill: process didn't stop gracefully", mp.name, pgid)
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		pm.log.Errorf("failed to send sigkill to '%s' (%d): %v", mp.name, pgid, err)
	}
}

func (pm *ProgramManager) initiateShutdown() {
	if !pm.terminating {
		pm.terminating = true
		for _, mp := range pm.Processes() {
			state := mp.State()
			if state == RUNNING || state == STARTING {
				pm.sendEvent(STOP, mp)
			}
		}
	}
}

func (pm *ProgramManager) allProcessesTerminated() bool {
	for _, mp := range pm.Processes() {
		state := mp.State()
		if state != STOPPED && state != EXITED && state != BACKOFF && state != FATAL {
			return false
		}
	}

	return true
}

func (pm *ProgramManager) StartProcess(processName string, replyChan chan<- RequestReply) {
	mp, ok := pm.GetProcess(processName)
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.State()
	switch state {
	case RUNNING, STARTING:
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("already %s", strings.ToLower(string(state))),
		}
	default:
		pm.SetRequest(processName, replyChan)
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
	mp, ok := pm.GetProcess(processName)
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.State()
	switch state {
	case STOPPED, EXITED, FATAL, BACKOFF:
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("already %s", strings.ToLower(string(state))),
		}
	default:
		pm.SetRequest(processName, replyChan)
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
	mp, ok := pm.GetProcess(processName)
	if !ok {
		replyChan <- RequestReply{
			Name: processName,
			Err:  fmt.Errorf("no such process"),
		}
		return
	}

	state := mp.State()
	if state == RUNNING {
		replyChan <- RequestReply{
			Name:    processName,
			Message: strconv.Itoa(mp.Cmd().Process.Pid),
		}
	} else {
		replyChan <- RequestReply{
			Name: processName,
			Err:  errors.New("not running"),
		}
	}
}

func (pm *ProgramManager) GetAllProcessPIDs(replyChan chan<- []RequestReply) {
	processes := pm.Processes()
	processCount := len(processes)
	processReplyChan := make(chan RequestReply, processCount)

	for processName := range processes {
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
	processes := pm.Processes()
	processCount := len(processes)
	processReplyChan := make(chan ProcessStatus, processCount)

	for processName := range processes {
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
	mp, ok := pm.GetProcess(processName)
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
	state := mp.State()
	if state == STARTING && mp.hasStartTimeoutExpired(pm.Config.StartSecs) {
		pm.sendEvent(PROCESS_STARTED, mp)
	} else if state == STOPPING {
		if mp.hasStopTimeoutExpired(pm.Config.StopSecs) {
			pm.forceStop(mp)
		} else {
			pm.log.Infof("waiting for '%s' to stop", mp.name)
		}
	} else if state == EXITED && mp.shouldRestart(pm.Config.AutoRestart, pm.Config.ExitCodes) && !pm.terminating {
		pm.sendEvent(START, mp)
	} else if state == BACKOFF && time.Now().After(mp.NextRestartTime()) && !pm.terminating {
		pm.sendEvent(START, mp)
	}
}

func (pm *ProgramManager) Run() {
	defer close(pm.doneChan)

	for processNum := range pm.Config.NumProcs {
		processName := pm.getProcessName(processNum)
		pm.SetProcess(processName, newManagedProcess(processNum, processName, pm.exitChan, pm.Config.StopSignal))

		if pm.Config.AutoStart {
			mp, _ := pm.GetProcess(processName)
			pm.sendEvent(START, mp)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopChan:
			pm.initiateShutdown()

			if pm.allProcessesTerminated() {
				return
			}
		case exit := <-pm.exitChan:
			mp := exit.Mp
			mp.SetExitTime(exit.ExitTime)

			state := mp.State()
			if state == STOPPING {
				pm.sendEvent(PROCESS_STOPPED, mp)

				pm.log.Infof("stopped: %s (%s)", mp.name, mp.stoppedSignal)
				pm.replyToRequest(mp, "stopped", false)
			} else {
				pm.sendEvent(PROCESS_EXITED, mp)

				exitCode := mp.Cmd().ProcessState.ExitCode()
				if mp.exitExpected(exitCode, pm.Config.ExitCodes, pm.Config.StartSecs) {
					pm.log.Infof("exited: %s (exit status %d; expected)", mp.name, exitCode)
				} else {
					pm.log.Warningf("exited: %s (exit status %d; not expected)", mp.name, exitCode)
				}

				pm.replyToRequest(mp, fmt.Sprintf("already %s", strings.ToLower(string(state))), true)
			}
		case <-ticker.C:
			for _, mp := range pm.Processes() {
				go pm.checkProcessState(mp)
			}
		}
	}
}

func (pm *ProgramManager) Stop() {
	close(pm.stopChan)
}

func (pm *ProgramManager) Wait() {
	<-pm.doneChan
}
