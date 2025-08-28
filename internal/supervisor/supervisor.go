package supervisor

import (
	"fmt"
	"os"
	"path/filepath"
	s "strings"
	"sync"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
	"taskmaster/internal/program"
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

	fmt.Printf("taskmaster: %+v\n\n", config.Taskmasterd)
	var wg sync.WaitGroup
	for programName, programConfig := range config.Programs {
		programManager := program.NewProgramManager(programName, &programConfig, config.Taskmasterd.ChildLogDir, log)

		wg.Add(1)
		go func(pm *program.ProgramManager) {
			defer wg.Done()

			if err := pm.Run(); err != nil {
				log.Warningf("program '%s' failed: %v", pm.Name, err)
			}
		}(programManager)
	}

	wg.Wait()

	return nil
}
