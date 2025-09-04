package controller

import (
	"fmt"
	"os"

	"github.com/chzyer/readline"
)

type Action string

const (
	HELP    Action = "help"
	START   Action = "start"
	STOP    Action = "stop"
	RESTART Action = "restart"
	STATUS  Action = "status"
	UPDATE  Action = "update"
	QUIT    Action = "quit"
	EXIT    Action = "exit"
)

type actionHandler func(ctl *Controller, lineFields []string)
type actionHelper func()

type actionMetadata struct {
	helper    actionHelper
	handler   actionHandler
	completer *readline.PrefixCompleter
}

func newActionMetadata(name string, helper actionHelper, handler actionHandler, pc readline.PrefixCompleterInterface) *actionMetadata {
	completers := []readline.PrefixCompleterInterface{}
	if pc != nil {
		completers = append(completers, pc)
	}

	return &actionMetadata{
		helper:    helper,
		handler:   handler,
		completer: readline.PcItem(name, completers...),
	}
}

func newActions(supervisor SupervisorInterface) map[Action]*actionMetadata {
	actions := map[Action]*actionMetadata{
		START:   newActionMetadata(string(START), startHelper, startAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		STOP:    newActionMetadata(string(STOP), stopHelper, stopAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		RESTART: newActionMetadata(string(RESTART), restartHelper, restartAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		STATUS:  newActionMetadata(string(STATUS), statusHelper, statusAction, readline.PcItemDynamic(supervisor.GetProgramNames())),
		UPDATE:  newActionMetadata(string(UPDATE), updateHelper, updateAction, nil),
		QUIT:    newActionMetadata(string(QUIT), quitHelper, shutdownAction, nil),
		EXIT:    newActionMetadata(string(EXIT), exitHelper, shutdownAction, nil),
	}

	actions[HELP] = newActionMetadata(string(HELP), helpHelper, helpAction, readline.PcItemDynamic(getActionNames(actions)))
	return actions
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

func startAction(ctl *Controller, lineFields []string) {
	if len(lineFields) >= 1 {
		for _, programName := range lineFields {
			if err := ctl.supervisor.StartProgram(programName); err != nil {
				fmt.Printf("*** %v\n", err)
			}
		}
	} else {
		// ctl.supervisor.StartAllPrograms()
	}
}

func stopAction(ctl *Controller, lineFields []string) {
	if len(lineFields) >= 1 {
		for _, programName := range lineFields {
			if err := ctl.supervisor.StopProgram(programName); err != nil {
				fmt.Printf("*** %v\n", err)
			}
		}
	} else {
		// ctl.supervisor.StartAllPrograms()
	}
}

func restartAction(ctl *Controller, lineFields []string) {
	if len(lineFields) >= 1 {
		for _, programName := range lineFields {
			if err := ctl.supervisor.RestartProgram(programName); err != nil {
				fmt.Printf("*** %v\n", err)
			}
		}
	} else {
		// ctl.supervisor.StartAllPrograms()
	}
}

func statusAction(ctl *Controller, lineFields []string) {
	if len(lineFields) > 1 {
		// ctl.supervisor.GetStatus(lineFields[1])
	} else {
		// ctl.supervisor.GetAllStatus()
	}
}

func updateAction(ctl *Controller, lineFields []string) {
	fmt.Println("Updating configuration")
}

func shutdownAction(ctl *Controller, lineFields []string) {
	ctl.running = false
}

func helpAction(ctl *Controller, lineFields []string) {
	if len(lineFields) == 0 {
		fmt.Println("┌───────────────── Available Actions ─────────────────┐")
		fmt.Println("│ Type 'help <action>'                                │")
		fmt.Println("└─────────────────────────────────────────────────────┘")

		actionNames := make([]string, 0, len(ctl.actions))
		for actionName := range ctl.actions {
			if actionName != HELP {
				actionNames = append(actionNames, string(actionName))
			}
		}

		for i, actionName := range actionNames {
			fmt.Printf("%-12s", actionName)
			if (i+1)%4 == 0 {
				fmt.Println()
			}
		}

		if len(actionNames)%4 != 0 {
			fmt.Println()
		}
	} else if len(lineFields) == 1 {
		action, ok := ctl.actions[Action(lineFields[0])]
		if !ok {
			fmt.Fprintf(os.Stderr, "*** no help available for '%v'\n", lineFields[0])
			fmt.Fprintln(os.Stderr, "*** type 'help' for a list of available actions")
			return
		}

		action.helper()
	} else {
		fmt.Fprintln(os.Stderr, "*** invalid help syntax. Use: help <action>")
	}
}

func startHelper() {
	fmt.Printf("start\t\t\tStart all processes\n")
	fmt.Printf("start <name>\t\tStart a process\n")
	fmt.Printf("start <name> <name>\tStart multiple processes\n")
}

func stopHelper() {
	fmt.Printf("stop\t\t\tStop all processes\n")
	fmt.Printf("stop <name>\t\tStop a process\n")
	fmt.Printf("stop <name> <name>\tStop multiple processes\n")
}

func restartHelper() {
	fmt.Printf("restart\t\t\tRestart all processes\n")
	fmt.Printf("restart <name>\t\tRestart a process\n")
	fmt.Printf("restart <name> <name>\tRestart multiple processes\n")
}

func statusHelper() {
	fmt.Printf("status\t\t\tShow status of all processes\n")
	fmt.Printf("status <name>\t\tShow status of a process\n")
	fmt.Printf("status <name> <name>\tShow status of multiple processes\n")
}

func updateHelper() {
	fmt.Printf("update\t\t\tReload configuration and update processes\n")
}

func quitHelper() {
	fmt.Printf("quit\t\tShutdown the supervisor\n")
}

func exitHelper() {
	fmt.Printf("exit\t\tShutdown the supervisor\n")
}

func helpHelper() {
	fmt.Printf("help\t\tPrint a list of available actions\n")
	fmt.Printf("help <action>\tPrint help for <action>\n")
}
