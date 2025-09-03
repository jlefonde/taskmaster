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

func NewEmbeddedController(supervisor SupervisorInterface) (*Controller, error) {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            "taskmaster> ",
		HistoryFile:       "/tmp/readline.tmp",
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		AutoComplete:      newCompleter(supervisor),
		HistorySearchFold: true,
	})
	if err != nil {
		return nil, err
	}

	return &Controller{
		supervisor: supervisor,
		rl:         rl,
	}, nil
}

func newCompleter(supervisor SupervisorInterface) *readline.PrefixCompleter {
	var programNames []readline.PrefixCompleterInterface
	for _, programName := range supervisor.GetProgramNames() {
		programNames = append(programNames, readline.PcItem(programName))
	}

	allPrograms := append(programNames, readline.PcItem("all"))

	actions := []*readline.PrefixCompleter{
		readline.PcItem("start", allPrograms...),
		readline.PcItem("stop", allPrograms...),
		readline.PcItem("restart", allPrograms...),
		readline.PcItem("status", allPrograms...),
		readline.PcItem("update"),
		readline.PcItem("shutdown"),
		readline.PcItem("reload"),
	}

	var helpCompletions []readline.PrefixCompleterInterface
	for _, action := range actions {
		helpCompletions = append(helpCompletions, readline.PcItem(string(action.Name)))
	}

	helpAction := readline.PcItem("help", helpCompletions...)

	var flatActions []readline.PrefixCompleterInterface
	flatActions = append(flatActions, helpAction)
	for _, action := range actions {
		flatActions = append(flatActions, action)
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
