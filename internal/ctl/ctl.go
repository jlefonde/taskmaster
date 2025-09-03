package ctl

import (
	"fmt"
	"io"
	"strings"

	"github.com/chzyer/readline"
)

type Action string

const (
	START    Action = "start"
	STOP     Action = "stop"
	RESTART  Action = "restart"
	STATUS   Action = "status"
	UPDATE   Action = "update"
	SHUTDOWN Action = "shutdown"
)

type Controller struct {
	supervisor SupervisorInterface
	rl         *readline.Instance
	actions    map[Action]*actionMetadata
}

type actionMetadata struct {
	description string
	completer   *readline.PrefixCompleter
}

type SupervisorInterface interface {
	GetProgramNames() []string
	// StartProgram(name string) error
	// StartAllPrograms() error
	// StopProgram(name string) error
	// StopAllPrograms() error
	// RestartProgram(name string) error
	// RestartAllPrograms() error
	// GetStatus(name string) (string, error)
	// GetAllStatus(name string) (string, error)
}

func newActions(supervisor SupervisorInterface) map[Action]*actionMetadata {
	var programNames []readline.PrefixCompleterInterface
	for _, programName := range supervisor.GetProgramNames() {
		programNames = append(programNames, readline.PcItem(programName))
	}

	return map[Action]*actionMetadata{
		START:    newActionMetadata(string(START), "", programNames...),
		STOP:     newActionMetadata(string(STOP), "", programNames...),
		RESTART:  newActionMetadata(string(RESTART), "", programNames...),
		STATUS:   newActionMetadata(string(STATUS), "", programNames...),
		UPDATE:   newActionMetadata(string(UPDATE), ""),
		SHUTDOWN: newActionMetadata(string(SHUTDOWN), ""),
	}
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

func newActionMetadata(name string, description string, pc ...readline.PrefixCompleterInterface) *actionMetadata {
	return &actionMetadata{
		description: description,
		completer:   readline.PcItem(name, pc...),
	}
}

func newCompleter(actions map[Action]*actionMetadata) *readline.PrefixCompleter {
	var helpCompletions []readline.PrefixCompleterInterface
	for actionName := range actions {
		helpCompletions = append(helpCompletions, readline.PcItem(string(actionName)))
	}

	helpAction := readline.PcItem("help", helpCompletions...)

	var flatActions []readline.PrefixCompleterInterface
	flatActions = append(flatActions, helpAction)
	for _, action := range actions {
		flatActions = append(flatActions, action.completer)
	}

	return readline.NewPrefixCompleter(flatActions...)
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
		fmt.Println(line)
	}

	return nil
}
