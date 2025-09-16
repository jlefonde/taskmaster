package supervisor

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/jlefonde/taskmaster/internal/config"
	"github.com/jlefonde/taskmaster/internal/program"
)

func (s *Supervisor) startAllPrograms(replyChan chan<- []program.RequestReply) {
	var replies []program.RequestReply

	for _, pm := range s.ProgramManagers() {
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
	pm, ok := s.getProgramManager(programName)
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				Name: processName,
				Err:  fmt.Errorf("no such process"),
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

	for _, pm := range s.ProgramManagers() {
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
	pm, ok := s.getProgramManager(programName)
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				Name: processName,
				Err:  fmt.Errorf("no such process"),
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

	for _, pm := range s.ProgramManagers() {
		programReplies := make(chan []program.RequestReply, 1)
		pm.GetAllProcessPIDs(programReplies)
		replies = append(replies, <-programReplies...)
	}

	replyChan <- replies
}

func (s *Supervisor) PidRequest(processName string, replyChan chan<- []program.RequestReply) {
	if processName == "" {
		replyChan <- []program.RequestReply{{
			Name:    "taskmasterd",
			Message: strconv.Itoa(os.Getpid())}}
		return
	}

	if processName == "all" {
		s.getAllProgramsProcessPids(replyChan)
		return
	}

	programName, processNameCut, sepFound := strings.Cut(processName, ":")
	pm, ok := s.getProgramManager(programName)
	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
		replyChan <- []program.RequestReply{
			{
				Name: processName,
				Err:  fmt.Errorf("no such process"),
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

	for _, pm := range s.ProgramManagers() {
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
	pm, ok := s.getProgramManager(programName)
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

func (s *Supervisor) removeProcessGroup(programName string, replyChan chan<- program.RequestReply) {
	replyChan <- program.RequestReply{
		Name:    programName,
		Message: "removed process group",
	}

	s.log.Info("removed: ", programName)
}

func (s *Supervisor) updateProcessGroup(programName string, programConfig *config.Program, replyChan chan<- program.RequestReply) {
	s.startProgramManager(programName, programConfig)

	replyChan <- program.RequestReply{
		Name:    programName,
		Message: "updated process group",
	}

	s.log.Info("updated: ", programName)
}

func (s *Supervisor) addProcessGroup(programName string, programConfig *config.Program, replyChan chan<- program.RequestReply) {
	s.startProgramManager(programName, programConfig)

	replyChan <- program.RequestReply{
		Name:    programName,
		Message: "added process group",
	}

	s.log.Info("added: ", programName)
}

func (s *Supervisor) UpdateRequest(replyChan chan<- program.RequestReply) {
	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()
	defer close(replyChan)

	newConfig, err := config.NewConfig(s.config.Path)
	if err != nil {
		replyChan <- program.RequestReply{
			Name: "",
			Err:  err,
		}

		s.log.Errorf("failed to update config: %v", err)
		return
	}

	if reflect.DeepEqual(newConfig.Programs, s.config.Programs) {
		s.log.Info("no configuration changes detected, skipping config update")
		return
	}

	processedPrograms := make(map[string]bool)

	for programName, pm := range s.ProgramManagers() {
		programConfig, ok := newConfig.Programs[programName]
		if ok && reflect.DeepEqual(&programConfig, pm.Config) {
			processedPrograms[programName] = true
			continue
		}

		pm.Stop()
		pm.Wait()
		s.deleteProgramManager(programName)

		replyChan <- program.RequestReply{
			Name:    programName,
			Message: "stopped",
		}

		if !ok {
			s.removeProcessGroup(programName, replyChan)
		} else {
			s.updateProcessGroup(programName, &programConfig, replyChan)
			processedPrograms[programName] = true
		}
	}

	for programName, programConfig := range newConfig.Programs {
		if !processedPrograms[programName] {
			s.addProcessGroup(programName, &programConfig, replyChan)
		}
	}

	s.config.Programs = newConfig.Programs
	s.log.Info("config successfully updated")
}
