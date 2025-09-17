package supervisor

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/jlefonde/taskmaster/internal/config"
	"github.com/jlefonde/taskmaster/internal/controller"
	"github.com/jlefonde/taskmaster/internal/logger"
	"github.com/jlefonde/taskmaster/internal/program"
)

type Supervisor struct {
	config          *config.Config
	log             *logger.Logger
	programManagers map[string]*program.ProgramManager
	quitSigs        chan os.Signal
	updateSigs      chan os.Signal
	ctlExited       chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	updateMutex     sync.Mutex
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
		wg:              sync.WaitGroup{},
		ctlExited:       make(chan struct{}),
	}, nil
}

func (s *Supervisor) ProgramManagers() map[string]*program.ProgramManager {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.programManagers
}

func (s *Supervisor) GetProgramManager(programName string) (*program.ProgramManager, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pm, ok := s.programManagers[programName]
	return pm, ok
}

func (s *Supervisor) SetProgramManager(programName string, pm *program.ProgramManager) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.programManagers[programName] = pm
}

func (s *Supervisor) DeleteProgramManager(programName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.programManagers, programName)
}

func cleanupLogFiles(log *logger.Logger, childLogDir string) error {
	logDir, err := os.ReadDir(childLogDir)
	if err != nil {
		return fmt.Errorf("read child log directory failed: %w", err)
	}

	for _, entry := range logDir {
		if entry.IsDir() || !strings.Contains(entry.Name(), "---taskmaster") {
			continue
		}

		if err := os.Remove(filepath.Join(childLogDir, entry.Name())); err != nil {
			log.Warning("remove log file failed: ", err)
		}
	}

	return nil
}

func (s *Supervisor) GetProcessNames() func(string) []string {
	return func(string) []string {
		var processNames []string
		for programName, pm := range s.ProgramManagers() {
			for processName := range pm.Processes() {
				processNames = append(processNames, processName)
			}

			if pm.Config.NumProcs > 1 {
				processNames = append(processNames, programName+":*")
			}
		}

		return processNames
	}
}

func (s *Supervisor) startProgramManager(programName string, programConfig *config.Program) {
	pm := program.NewProgramManager(programName, programConfig, s.config.Taskmasterd.ChildLogDir, s.log)
	s.SetProgramManager(programName, pm)

	s.wg.Add(1)
	go func(pm *program.ProgramManager) {
		defer s.wg.Done()

		pm.Run()
	}(pm)
}

func (s *Supervisor) updateConfig() {
	replyChan := make(chan program.RequestReply)

	go func() {
		s.UpdateRequest(replyChan)
	}()

	for range replyChan {
	}
}

func (s *Supervisor) Wait() {
	for {
		select {
		case sig := <-s.updateSigs:
			s.log.Infof("received signal: %s, updating config", sig)
			s.updateConfig()
		case <-s.ctlExited:
			s.log.Info("controller exited, shutting down supervisor")
			return
		case sig := <-s.quitSigs:
			s.log.Infof("received signal: %s, shutting down supervisor", sig)
			return
		}
	}
}

func (s *Supervisor) Stop() {
	for _, pm := range s.ProgramManagers() {
		go pm.Stop()
	}

	s.wg.Wait()

	if err := s.log.CloseLogFile(); err != nil {
		s.log.Warning("failed to close log file: ", err)
	}
}

func (s *Supervisor) Run() {
	s.log.Info("taskmasterd started with pid ", os.Getpid())

	if !s.config.Taskmasterd.NoCleanup {
		if err := cleanupLogFiles(s.log, s.config.Taskmasterd.ChildLogDir); err != nil {
			s.log.Warning("couldn't cleanup log files: ", err)
		}
	}

	s.quitSigs = make(chan os.Signal, 1)
	s.updateSigs = make(chan os.Signal, 1)
	signal.Notify(s.quitSigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	signal.Notify(s.updateSigs, syscall.SIGHUP)

	for programName, programConfig := range s.config.Programs {
		s.startProgramManager(programName, &programConfig)
	}

	ctl, err := controller.NewEmbeddedController(s, s.log)
	if err != nil {
		s.log.Critical("failed to create controller: ", err)

		s.Stop()
		os.Exit(1)
	}

	go func() {
		defer close(s.ctlExited)

		ctl.Start()
	}()

	s.Wait()
	s.Stop()
}
