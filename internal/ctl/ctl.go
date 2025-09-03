package ctl

import (
	"fmt"
	"io"
	"strings"

	"github.com/chzyer/readline"
)

type Action string

const (
	HELP     Action = "help"
	START    Action = "start"
	STOP     Action = "stop"
	RESTART  Action = "restart"
	STATUS   Action = "status"
	UPDATE   Action = "update"
	SHUTDOWN Action = "shutdown"
)

type actionHandler func([]string)

type actionMetadata struct {
	description string
	handler     actionHandler
	completer   *readline.PrefixCompleter
}

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

func startAction(lineFields []string) {
	if len(lineFields) > 1 {
		fmt.Printf("Starting program: %s\n", lineFields[1])
		// ctl.supervisor.StartProgram(lineFields[1])
	} else {
		fmt.Println("Starting all programs")
		// ctl.supervisor.StartAllPrograms()
	}
}

func getActionNames(actions map[Action]*actionMetadata) func(string) []string {
	return func(string) []string {
		actionNames := make([]string, 0)
		for actionName := range actions {
			actionNames = append(actionNames, string(actionName))
		}

		return actionNames
	}
}

func newActions(supervisor SupervisorInterface) map[Action]*actionMetadata {
	actions := map[Action]*actionMetadata{
		START:    newActionMetadata(string(START), "", startAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		STOP:     newActionMetadata(string(STOP), "", startAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		RESTART:  newActionMetadata(string(RESTART), "", startAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		STATUS:   newActionMetadata(string(STATUS), "", startAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		UPDATE:   newActionMetadata(string(UPDATE), "", startAction, nil),
		SHUTDOWN: newActionMetadata(string(SHUTDOWN), "", startAction, nil),
	}

	actions[HELP] = newActionMetadata(string(HELP), "", startAction, readline.PcItemDynamic(getActionNames(actions)))
	return actions
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

func newActionMetadata(name string, description string, handler actionHandler, pc readline.PrefixCompleterInterface) *actionMetadata {
	completers := []readline.PrefixCompleterInterface{}
	if pc != nil {
		completers = append(completers, pc)
	}

	return &actionMetadata{
		description: description,
		handler:     handler,
		completer:   readline.PcItem(name, completers...),
	}
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

		action.handler(lineFields)
	}

	return nil
}
