package config

import (
	"errors"
	"os"
	"path/filepath"
)

func ReadConfigFile(configPath string) ([]byte, error) {
	if configPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		executablePath, err := os.Executable()
		if err != nil {
			return nil, err
		}

		dir := filepath.Dir(executablePath)

		defaultLocations := []string{
			filepath.Join(dir, "../etc/taskmasterd.yaml"),
			filepath.Join(dir, "../taskmasterd.yaml"),
			filepath.Join(cwd, "/taskmasterd.yaml"),
			filepath.Join(cwd, "/etc/taskmasterd.yaml"),
			"/etc/taskmasterd.yaml",
			"/etc/taskmaster/taskmasterd.conf",
		}

		var conf []byte
		for _, location := range defaultLocations {
			conf, err = os.ReadFile(location)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				} else {
					return nil, err
				}
			}
			break
		}

		if conf == nil {
			return nil, errors.New("no configuration file found in default locations")
		}

		return conf, nil
	}

	conf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	return conf, nil
}
