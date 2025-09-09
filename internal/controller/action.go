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
		START:   newActionMetadata(string(START), startHelper, startAction, readline.PcItemDynamic(supervisor.GetProcessNames())),
		STOP:    newActionMetadata(string(STOP), stopHelper, stopAction, readline.PcItemDynamic(supervisor.GetProcessNames())),
		RESTART: newActionMetadata(string(RESTART), restartHelper, restartAction, readline.PcItemDynamic(supervisor.GetProcessNames())),
		STATUS:  newActionMetadata(string(STATUS), statusHelper, statusAction, readline.PcItemDynamic(supervisor.GetProcessNames())),
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
	if len(lineFields) == 0 {
		fmt.Fprintln(os.Stderr, "*** invalid start syntax")
		startHelper()
		return
	}

	for _, processName := range lineFields {
		replyChan := make(chan string, 1)
		ctl.supervisor.StartRequest(processName, replyChan)

		reply := <-replyChan
		fmt.Println(reply)
	}
}

func stopAction(ctl *Controller, lineFields []string) {
	if len(lineFields) == 0 {
		fmt.Fprintln(os.Stderr, "*** invalid stop syntax")
		stopHelper()
		return
	}

	for _, processName := range lineFields {
		replyChan := make(chan string, 1)
		ctl.supervisor.StopRequest(processName, replyChan)

		reply := <-replyChan
		fmt.Println(reply)
	}
}

func restartAction(ctl *Controller, lineFields []string) {

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
		fmt.Println("┌────────────────────── Available Actions ─────────────────────┐")
		fmt.Println("│ Type 'help <action>'                                         │")
		fmt.Println("└──────────────────────────────────────────────────────────────┘")

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
			fmt.Fprintln(os.Stderr, "*** no help available for", lineFields[0])
			fmt.Fprintln(os.Stderr, "*** type 'help' for a list of available actions")
			return
		}

		action.helper()
	} else {
		fmt.Fprintln(os.Stderr, "*** invalid help syntax. Use: help <action>")
	}
}

func startHelper() {
	fmt.Println("start all\t\tStart all processes")
	fmt.Println("start <name>\t\tStart a process")
	fmt.Println("start <name> <name>\tStart multiple processes")
}

func stopHelper() {
	fmt.Println("stop all\t\tStop all processes")
	fmt.Println("stop <name>\t\tStop a process")
	fmt.Println("stop <name> <name>\tStop multiple processes")
}

func restartHelper() {
	fmt.Println("restart all\t\tRestart all processes")
	fmt.Println("restart <name>\t\tRestart a process")
	fmt.Println("restart <name> <name>\tRestart multiple processes")
}

func statusHelper() {
	fmt.Println("status all\t\tShow status of all processes")
	fmt.Println("status <name>\t\tShow status of a process")
	fmt.Println("status <name> <name>\tShow status of multiple processes")
}

func updateHelper() {
	fmt.Println("update\t\t\tReload configuration and update processes")
}

func quitHelper() {
	fmt.Println("quit\t\tShutdown the supervisor")
}

func exitHelper() {
	fmt.Println("exit\t\tShutdown the supervisor")
}

func helpHelper() {
	fmt.Println("help\t\tPrint a list of available actions")
	fmt.Println("help <action>\tPrint help for <action>")
}
