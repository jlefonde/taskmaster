package supervisor

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"taskmaster/internal/config"
	"taskmaster/internal/program"
)

func (s *Supervisor) startAllPrograms(replyChan chan<- []program.RequestReply) {
	var replies []program.RequestReply

	for _, pm := range s.programManagers {
		programReplies := make(chan []program.RequestReply, 1)
		pm.StartAllProcesses(programReplies)
		replies = append(replies, <-programReplies...)
	}

	replyChan <- replies
}

func (s *Supervisor) StartRequest(processName string, replyChan chan<- []program.RequestReply) {
	if processName == "all" {
		s.startAllPrograms(replyChan)
		return
	}

	programName, processNameCut, sepFound := strings.Cut(processName, ":")
	pm, ok := s.programManagers[programName]
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				ProcessName: processName,
				Err:         fmt.Errorf("no such process"),
			},
		}
		return
	}

	if (processNameCut == "" || processNameCut == "*") && pm.Config.NumProcs > 1 {
		pm.StartAllProcesses(replyChan)
		return
	}

	reply := make(chan program.RequestReply, 1)
	pm.StartProcess(processName, reply)
	replyChan <- []program.RequestReply{<-reply}
}

func (s *Supervisor) stopAllPrograms(replyChan chan<- []program.RequestReply) {
	var replies []program.RequestReply

	for _, pm := range s.programManagers {
		programReplies := make(chan []program.RequestReply, 1)
		pm.StopAllProcesses(programReplies)
		replies = append(replies, <-programReplies...)
	}

	replyChan <- replies
}

func (s *Supervisor) StopRequest(processName string, replyChan chan<- []program.RequestReply) {
	if processName == "all" {
		s.stopAllPrograms(replyChan)
		return
	}

	programName, processNameCut, sepFound := strings.Cut(processName, ":")
	pm, ok := s.programManagers[programName]
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				ProcessName: processName,
				Err:         fmt.Errorf("no such process"),
			},
		}
		return
	}

	if (processNameCut == "" || processNameCut == "*") && pm.Config.NumProcs > 1 {
		pm.StopAllProcesses(replyChan)
		return
	}

	processReplychan := make(chan program.RequestReply, 1)
	pm.StopProcess(processName, processReplychan)
	replyChan <- []program.RequestReply{<-processReplychan}
}

func (s *Supervisor) getAllProgramsProcessPids(replyChan chan<- []program.RequestReply) {
	var replies []program.RequestReply

	for _, pm := range s.programManagers {
		programReplies := make(chan []program.RequestReply, 1)
		pm.GetAllProcessPIDs(programReplies)
		replies = append(replies, <-programReplies...)
	}

	replyChan <- replies
}

func (s *Supervisor) PidRequest(processName string, replyChan chan<- []program.RequestReply) {
	if processName == "" {
		replyChan <- []program.RequestReply{{
			ProcessName: "taskmasterd",
			Message:     strconv.Itoa(os.Getpid())}}
		return
	}

	if processName == "all" {
		s.getAllProgramsProcessPids(replyChan)
		return
	}

	programName, processNameCut, sepFound := strings.Cut(processName, ":")
	pm, ok := s.programManagers[programName]
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				ProcessName: processName,
				Err:         fmt.Errorf("no such process"),
			},
		}
		return
	}

	if (processNameCut == "" || processNameCut == "*") && pm.Config.NumProcs > 1 {
		pm.GetAllProcessPIDs(replyChan)
		return
	}

	reply := make(chan program.RequestReply, 1)
	pm.GetProcessPID(processName, reply)
	replyChan <- []program.RequestReply{<-reply}
}

func (s *Supervisor) getAllProgramsStatus(replyChan chan<- []program.ProcessStatus) {
	var replies []program.ProcessStatus

	for _, pm := range s.programManagers {
		programReplies := make(chan []program.ProcessStatus, 1)
		pm.GetAllProcessesStatus(programReplies)
		replies = append(replies, <-programReplies...)
	}

	replyChan <- replies
}

func (s *Supervisor) StatusRequest(processName string, replyChan chan<- []program.ProcessStatus) {
	if processName == "all" {
		s.getAllProgramsStatus(replyChan)
		return
	}

	programName, processNameCut, sepFound := strings.Cut(processName, ":")
	pm, ok := s.programManagers[programName]
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.ProcessStatus{
			{
				Name: processName,
				Err:  fmt.Errorf("no such process"),
			},
		}
		return
	}

	if (processNameCut == "" || processNameCut == "*") && pm.Config.NumProcs > 1 {
		pm.GetAllProcessesStatus(replyChan)
		return
	}

	processReplychan := make(chan program.ProcessStatus, 1)
	pm.GetProcessStatus(processName, processReplychan)
	replyChan <- []program.ProcessStatus{<-processReplychan}
}

func (s *Supervisor) UpdateRequest(replyChan chan<- program.RequestReply) {
	s.configMutex.Lock()
	defer s.configMutex.Unlock()
	defer close(replyChan)

	s.log.Info("updating config")

	newConfig, err := config.NewConfig(s.config.Path)
	if err != nil {
		replyChan <- program.RequestReply{
			ProcessName: "",
			Err:         err,
		}

		s.log.Errorf("failed to update config: %v", err)
		return
	}

	if reflect.DeepEqual(newConfig.Programs, s.config.Programs) {
		s.log.Info("no configuration changes detected, skipping config update")
		return
	}

	processedPrograms := make(map[string]bool)

	for programName, pm := range s.programManagers {
		programConfig, ok := newConfig.Programs[programName]
		if ok && reflect.DeepEqual(&programConfig, pm.Config) {
			processedPrograms[programName] = true
			continue
		}

		pm.Stop()
		pm.Wait()
		delete(s.programManagers, pm.Name)

		replyChan <- program.RequestReply{
			ProcessName: pm.Name,
			Message:     "stopped",
		}

		if !ok {
			replyChan <- program.RequestReply{
				ProcessName: pm.Name,
				Message:     "removed process group",
			}

			s.log.Info("removed: ", pm.Name)
		} else {
			s.startProgramManager(programName, &programConfig)

			replyChan <- program.RequestReply{
				ProcessName: pm.Name,
				Message:     "updated process group",
			}

			s.log.Info("updated: ", pm.Name)

			processedPrograms[programName] = true
		}
	}

	for programName, programConfig := range newConfig.Programs {
		if !processedPrograms[programName] {
			s.startProgramManager(programName, &programConfig)

			replyChan <- program.RequestReply{
				ProcessName: programName,
				Message:     "added process group",
			}

			s.log.Info("added: ", programName)
		}
	}

	s.config.Programs = newConfig.Programs
	s.log.Info("config successfully updated")
}
