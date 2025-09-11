package logger

import (
	"fmt"
	"log"
	"os"
)

type LogLevel int

const (
	FATAL LogLevel = iota
	ERROR
	WARNING
	INFO
	DEBUG
	UNKNOWN
	NONE
)

type Logger struct {
	logger    *log.Logger
	level     LogLevel
	logLevels []string
}

func NewLogger(logFilePath string, logLevel LogLevel) (*Logger, error) {
	// TODO: replace os.O_TRUNC by os.O_APPEND
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("open logfile failed: %w", err)
	}

	return &Logger{log.New(logFile, "", log.Lmsgprefix|log.Ldate|log.Ltime), logLevel, []string{
		FATAL:   "FATAL   ",
		ERROR:   "ERROR   ",
		WARNING: "WARNING ",
		INFO:    "INFO    ",
		DEBUG:   "DEBUG   ",
	}}, nil
}

func (l *Logger) Fatalf(format string, v ...any) {
	if l.level <= FATAL {
		l.logger.Fatalf(l.logLevels[FATAL]+format, v...)
	}
}

func (l *Logger) Errorf(format string, v ...any) {
	if l.level <= ERROR {
		l.logger.Printf(l.logLevels[ERROR]+format, v...)
	}
}

func (l *Logger) Warningf(format string, v ...any) {
	if l.level <= WARNING {
		l.logger.Printf(l.logLevels[WARNING]+format, v...)
	}
}

func (l *Logger) Infof(format string, v ...any) {
	if l.level <= INFO {
		l.logger.Printf(l.logLevels[INFO]+format, v...)
	}
}

func (l *Logger) Debugf(format string, v ...any) {
	if l.level <= DEBUG {
		l.logger.Printf(l.logLevels[DEBUG]+format, v...)
	}
}

func (l *Logger) Fatal(v ...any) {
	if l.level <= FATAL {
		message := fmt.Sprint(v...)
		l.logger.Println(l.logLevels[FATAL] + message)
	}
}

func (l *Logger) Error(v ...any) {
	if l.level <= ERROR {
		message := fmt.Sprint(v...)
		l.logger.Println(l.logLevels[ERROR] + message)
	}
}

func (l *Logger) Warning(v ...any) {
	if l.level <= WARNING {
		message := fmt.Sprint(v...)
		l.logger.Println(l.logLevels[WARNING] + message)
	}
}

func (l *Logger) Info(v ...any) {
	if l.level <= INFO {
		message := fmt.Sprint(v...)
		l.logger.Println(l.logLevels[INFO] + message)
	}
}

func (l *Logger) Debug(v ...any) {
	if l.level <= DEBUG {
		message := fmt.Sprint(v...)
		l.logger.Println(l.logLevels[DEBUG] + message)
	}
}
