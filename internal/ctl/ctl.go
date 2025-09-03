package ctl

import (
	"fmt"
	"io"
	"strings"

	"github.com/chzyer/readline"
)

type Controller struct {
	supervisor SupervisorInterface
	rl         *readline.Instance
	actions    map[Action]*actionMetadata
}

type SupervisorInterface interface {
	GetProgramNames() func(string) []string
	// StartProgram(name string) error
	// StartAllPrograms() error
	// StopProgram(name string) error
	// StopAllPrograms() error
	// RestartProgram(name string) error
	// RestartAllPrograms() error
	// GetStatus(name string) (string, error)
	// GetAllStatus(name string) (string, error)
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
	}, nil
}

func newCompleter(actions map[Action]*actionMetadata) *readline.PrefixCompleter {
	var allActions []readline.PrefixCompleterInterface
	for _, action := range actions {
		allActions = append(allActions, action.completer)
	}

	return readline.NewPrefixCompleter(allActions...)
}

func (ctl *Controller) Start() error {
	defer ctl.rl.Close()

	ctl.rl.CaptureExitSignal()

	for {
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
			fmt.Printf("Unknown command: %s\n", actionName)
			continue
		}

		action.handler(ctl, lineFields)
	}

	return nil
}
