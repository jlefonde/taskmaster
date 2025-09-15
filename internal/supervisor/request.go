package supervisor

import (
	"fmt"
	"reflect"
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

func (s *Supervisor) UpdateRequest(replyChan chan<- []program.RequestReply) {
	newConfig, err := config.NewConfig(s.config.Path)
	if err != nil {
		replyChan <- []program.RequestReply{
			{
				ProcessName: "",
				Err:         err,
			},
		}
		return
	}

	if reflect.DeepEqual(newConfig.Programs, s.config.Programs) {
		replyChan <- []program.RequestReply{}
		return
	}

	var stoppedManagers []*program.ProgramManager
	for programName, pm := range s.programManagers {
		programConfig, ok := newConfig.Programs[programName]
		if !ok || !reflect.DeepEqual(&programConfig, pm.Config) {
			pm.Stop()
			stoppedManagers = append(stoppedManagers, pm)
		}
	}

	var replies []program.RequestReply
	for _, pm := range stoppedManagers {
		pm.Wait()

		replies = append(replies, program.RequestReply{
			ProcessName: pm.Name,
			Message:     "stopped",
		})

		delete(s.programManagers, pm.Name)

		replies = append(replies, program.RequestReply{
			ProcessName: pm.Name,
			Message:     "removed process group",
		})

	}

	for programName, programConfig := range newConfig.Programs {
		if _, ok := s.programManagers[programName]; !ok {
			s.programManagers[programName] = program.NewProgramManager(programName, &programConfig, s.config.Taskmasterd.ChildLogDir, s.log)

			s.wg.Add(1)
			go func(pm *program.ProgramManager) {
				defer s.wg.Done()

				pm.Run()
			}(s.programManagers[programName])

			replies = append(replies, program.RequestReply{
				ProcessName: programName,
				// TODO: diff between added and updated
				Message: "added process group",
			})
		}
	}

	s.config = newConfig
	replyChan <- replies
}
