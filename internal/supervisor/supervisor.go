package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	s "strings"
	"sync"
	"syscall"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
	"taskmaster/internal/program"

	"github.com/chzyer/readline"
)

func cleanupLogFiles(log *logger.Logger, childLogDir string) error {
	logDir, err := os.ReadDir(childLogDir)
	if err != nil {
		return fmt.Errorf("read child log directory failed: %w", err)
	} else {
		for _, entry := range logDir {
			if entry.IsDir() || !s.Contains(entry.Name(), "---taskmaster") {
				continue
			}

			if err := os.Remove(filepath.Join(childLogDir, entry.Name())); err != nil {
				log.Warning("remove log file failed:", err)
			}
		}
	}

	return nil
}

func Run(config *config.Config) error {
	log, err := logger.CreateLogger(config.Taskmasterd.LogFile, config.Taskmasterd.LogLevel)
	if err != nil {
		return fmt.Errorf("couldn't create logger: %w", err)
	}

	if !config.Taskmasterd.NoCleanup {
		if err := cleanupLogFiles(log, config.Taskmasterd.ChildLogDir); err != nil {
			log.Warning("couldn't cleanup log files:", err)
		}
	}

	log.Debugf("taskmaster: %+v\n\n", config.Taskmasterd)

	quitSigs := make(chan os.Signal, 1)
	signal.Notify(quitSigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	programManagers := make(map[string]*program.ProgramManager)

	var wg sync.WaitGroup
	for programName, programConfig := range config.Programs {
		programManagers[programName] = program.NewProgramManager(programName, &programConfig, config.Taskmasterd.ChildLogDir, log)

		wg.Add(1)
		go func(pm *program.ProgramManager) {
			defer wg.Done()

			pm.Run()
		}(programManagers[programName])
	}

	go func() {
		sig := <-quitSigs
		log.Debugf("Received %s", sig)
		for _, pm := range programManagers {
			go func(pm *program.ProgramManager) {
				log.Debugf("Stopping program manager: %s", pm.Name)
				pm.Stop()
			}(pm)
		}
	}()

	ctl, err := readline.NewEx(&readline.Config{
		Prompt:            "taskmaster> ",
		HistoryFile:       "/tmp/readline.tmp",
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
	if err != nil {
		// TODO: handle more gracefully
		panic(err)
	}

	defer ctl.Close()

	ctl.CaptureExitSignal()

	for {
		line, err := ctl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = s.TrimSpace(line)
		fmt.Println(line)
	}

	wg.Wait()

	return nil
}
