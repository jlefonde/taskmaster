package supervisor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"taskmaster/internal/config"
	"taskmaster/internal/ctl"
	"taskmaster/internal/logger"
	"taskmaster/internal/program"
)

type Supervisor struct {
	config          *config.Config
	log             *logger.Logger
	programManagers map[string]*program.ProgramManager
}

func NewSupervisor(config *config.Config) (*Supervisor, error) {
	log, err := logger.NewLogger(config.Taskmasterd.LogFile, config.Taskmasterd.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("couldn't create logger: %w", err)
	}

	return &Supervisor{
		config:          config,
		log:             log,
		programManagers: make(map[string]*program.ProgramManager),
	}, nil
}

func cleanupLogFiles(log *logger.Logger, childLogDir string) error {
	logDir, err := os.ReadDir(childLogDir)
	if err != nil {
		return fmt.Errorf("read child log directory failed: %w", err)
	} else {
		for _, entry := range logDir {
			if entry.IsDir() || !strings.Contains(entry.Name(), "---taskmaster") {
				continue
			}

			if err := os.Remove(filepath.Join(childLogDir, entry.Name())); err != nil {
				log.Warning("remove log file failed:", err)
			}
		}
	}

	return nil
}

func (s *Supervisor) GetProgramNames() []string {
	var programNames []string
	for programName := range s.programManagers {
		programNames = append(programNames, programName)
	}

	return programNames
}

func (s *Supervisor) Run() {
	if !s.config.Taskmasterd.NoCleanup {
		if err := cleanupLogFiles(s.log, s.config.Taskmasterd.ChildLogDir); err != nil {
			s.log.Warning("couldn't cleanup log files:", err)
		}
	}

	s.log.Debugf("taskmaster: %+v\n\n", s.config.Taskmasterd)

	quitSigs := make(chan os.Signal, 1)
	signal.Notify(quitSigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	var wg sync.WaitGroup
	for programName, programConfig := range s.config.Programs {
		s.programManagers[programName] = program.NewProgramManager(programName, &programConfig, s.config.Taskmasterd.ChildLogDir, s.log)

		wg.Add(1)
		go func(pm *program.ProgramManager) {
			defer wg.Done()

			pm.Run()
		}(s.programManagers[programName])
	}

	go func() {
		sig := <-quitSigs
		s.log.Debugf("Received %s", sig)
		for _, pm := range s.programManagers {
			go func(pm *program.ProgramManager) {
				s.log.Debugf("Stopping program manager: %s", pm.Name)
				pm.Stop()
			}(pm)
		}
	}()

	ctl, err := ctl.NewEmbeddedController(s)
	if err != nil {
		// TODO
	} else if err := ctl.Start(); err != nil {
		// TODO
	}

	wg.Wait()
}
