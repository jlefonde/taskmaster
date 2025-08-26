package logger

import (
	"fmt"
	"log"
	"os"
)

type LogLevel string

const (
	FATAL   LogLevel = "FATAL   "
	ERROR   LogLevel = "ERROR   "
	WARNING LogLevel = "WARNING "
	INFO    LogLevel = "INFO    "
	DEBUG   LogLevel = "DEBUG   "
	UNKNOWN LogLevel = ""
)

type Logger struct {
	logger *log.Logger
	level  LogLevel
}

func CreateLogger(LogFilePath string, logLevel LogLevel) (*Logger, error) {
	LogFile, err := os.OpenFile(LogFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open logfile failed: %w", err)
	}

	return &Logger{log.New(LogFile, "", log.Lmsgprefix|log.Ldate|log.Ltime), logLevel}, nil
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.logger.Fatalf(string(FATAL)+format, v...)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logger.Printf(string(ERROR)+format, v...)
}

func (l *Logger) Warningf(format string, v ...interface{}) {
	l.logger.Printf(string(WARNING)+format, v...)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	l.logger.Printf(string(INFO)+format, v...)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logger.Printf(string(DEBUG)+format, v...)
}

func (l *Logger) Fatal(v ...any) {
	args := append([]any{string(FATAL)}, v...)
	l.logger.Fatalln(args...)
}

func (l *Logger) Error(v ...any) {
	args := append([]any{string(ERROR)}, v...)
	l.logger.Println(args...)
}

func (l *Logger) Warning(v ...any) {
	args := append([]any{string(WARNING)}, v...)
	l.logger.Println(args...)
}

func (l *Logger) Info(v ...any) {
	args := append([]any{string(INFO)}, v...)
	l.logger.Println(args...)
}

func (l *Logger) Debug(v ...any) {
	args := append([]any{string(DEBUG)}, v...)
	l.logger.Println(args...)
}
