package controller

import (
	"fmt"
	"io"
	"os"
	"strings"

	"taskmaster/internal/program"

	"github.com/chzyer/readline"
)

type Controller struct {
	supervisor SupervisorInterface
	rl         *readline.Instance
	actions    map[Action]*actionMetadata
	running    bool
}

type SupervisorInterface interface {
	GetProcessNames() func(string) []string
	StartRequest(processName string, replyChan chan<- []program.RequestReply)
	StopRequest(processName string, replyChan chan<- []program.RequestReply)
	PidRequest(processName string, replyChan chan<- []program.RequestReply)
	StatusRequest(processName string, replyChan chan<- []program.ProcessStatus)
	UpdateRequest(replyChan chan<- program.RequestReply)
}

func NewEmbeddedController(supervisor SupervisorInterface) (*Controller, error) {
	actions := newActions(supervisor)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "taskmaster> ",
		HistoryFile:       "/tmp/readline.tmp",
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		AutoComplete:      newCompleter(actions),
		HistorySearchFold: true,
	})
	if err != nil {
		return nil, err
	}

	return &Controller{
		supervisor: supervisor,
		rl:         rl,
		actions:    actions,
		running:    true,
	}, nil
}

func newCompleter(actions map[Action]*actionMetadata) *readline.PrefixCompleter {
	var allActions []readline.PrefixCompleterInterface
	for _, action := range actions {
		allActions = append(allActions, action.completer)
	}

	return readline.NewPrefixCompleter(allActions...)
}

func (ctl *Controller) Start() {
	defer ctl.rl.Close()

	for ctl.running {
		line, err := ctl.rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		lineFields := strings.Fields(line)
		actionName := Action(lineFields[0])

		action, ok := ctl.actions[actionName]
		if !ok {
			fmt.Fprintln(os.Stderr, "*** unknown syntax:", actionName)
			continue
		}

		if err := action.handler(ctl, lineFields[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "*** invalid %s syntax\n", actionName)
			action.helper()
		}
	}
}
