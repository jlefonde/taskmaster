package supervisor

import (
	"fmt"
	"strings"

	"taskmaster/internal/program"
)

func (s *Supervisor) startAllPrograms(replyChan chan<- []program.RequestReply) {
	programReplyChan := make(chan []program.RequestReply, len(s.programManagers))

	for _, pm := range s.programManagers {
		pm.StartAllProcesses(programReplyChan)
	}

	var replies []program.RequestReply
	for range len(s.programManagers) {
		replies = append(replies, <-programReplyChan...)
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
				Err:         fmt.Errorf("no such processs"),
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
	programReplyChan := make(chan []program.RequestReply, len(s.programManagers))

	for _, pm := range s.programManagers {
		pm.StopAllProcesses(programReplyChan)
	}

	var replies []program.RequestReply
	for range len(s.programManagers) {
		replies = append(replies, <-programReplyChan...)
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
				Err:         fmt.Errorf("no such processs"),
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

// func (s *Supervisor) StatusRequest(processName string, replyChan chan<- string) {
// 	if processName == "all" {
// 		// TODO
// 		return
// 	}

// 	programName, processNameCut, sepFound := strings.Cut(processName, ":")
// 	pm, ok := s.programManagers[programName]
// 	if !ok || (pm.Config.NumProcs > 1 && !sepFound) {
// 		replyChan <- fmt.Sprintf("%s: ERROR (no such process)", processName)
// 		return
// 	}

// 	if (processNameCut == "" || processNameCut == "*") && pm.Config.NumProcs > 1 {
// 		// TODO
// 		return
// 	}

// 	status := pm.GetProcessStatus(processName, replyChan)
// 	replyChan <- status.Description
// }
