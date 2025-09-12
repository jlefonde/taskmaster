package logger

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
)

const (
	LOG_NONE    syslog.Priority = 1000
	LOG_UNKNOWN syslog.Priority = 2000

	SYSLOG string = "syslog"
)

type Logger struct {
	logger    *log.Logger
	level     syslog.Priority
	logLevels []string
	syslog    *syslog.Writer
}

func NewLogger(logFilePath string, logLevel syslog.Priority) (*Logger, error) {
	logLevels := []string{
		syslog.LOG_CRIT:    "CRITICAL ",
		syslog.LOG_ERR:     "ERROR    ",
		syslog.LOG_WARNING: "WARNING  ",
		syslog.LOG_INFO:    "INFO     ",
		syslog.LOG_DEBUG:   "DEBUG    ",
	}

	if logFilePath != SYSLOG {
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			return nil, fmt.Errorf("open logfile failed: %w", err)
		}

		return &Logger{
			logger:    log.New(logFile, "", log.Lmsgprefix|log.Ldate|log.Ltime),
			level:     logLevel,
			logLevels: logLevels,
		}, nil
	}

	logger, err := syslog.NewLogger(logLevel, log.Lmsgprefix|log.Ldate|log.Ltime)
	if err != nil {
		return nil, err
	}

	syslog, err := syslog.Dial("unixgram", "/dev/log", syslog.LOG_WARNING|syslog.LOG_USER, "taskmasterd")
	if err != nil {
		return nil, err
	}

	return &Logger{
		logger:    logger,
		level:     logLevel,
		logLevels: logLevels,
		syslog:    syslog,
	}, nil
}

func (l *Logger) Criticalf(format string, v ...any) {
	msg := fmt.Sprintf(l.logLevels[syslog.LOG_CRIT]+format, v...)

	if l.syslog != nil {
		l.syslog.Crit(msg)
		return
	}

	if l.level <= syslog.LOG_CRIT {
		l.logger.Print(msg)
	}
}

func (l *Logger) Errorf(format string, v ...any) {
	msg := fmt.Sprintf(l.logLevels[syslog.LOG_ERR]+format, v...)

	if l.syslog != nil {
		l.syslog.Err(msg)
		return
	}

	if l.level <= syslog.LOG_ERR {
		l.logger.Print(msg)
	}
}

func (l *Logger) Warningf(format string, v ...any) {
	msg := fmt.Sprintf(l.logLevels[syslog.LOG_WARNING]+format, v...)

	if l.syslog != nil {
		l.syslog.Warning(msg)
		return
	}

	if l.level <= syslog.LOG_WARNING {
		l.logger.Print(msg)
	}
}

func (l *Logger) Infof(format string, v ...any) {
	msg := fmt.Sprintf(l.logLevels[syslog.LOG_INFO]+format, v...)

	if l.syslog != nil {
		l.syslog.Info(msg)
		return
	}

	if l.level <= syslog.LOG_INFO {
		l.logger.Print(msg)
	}
}

func (l *Logger) Debugf(format string, v ...any) {
	msg := fmt.Sprintf(l.logLevels[syslog.LOG_DEBUG]+format, v...)

	if l.syslog != nil {
		l.syslog.Debug(msg)
		return
	}

	if l.level <= syslog.LOG_DEBUG {
		l.logger.Print(msg)
	}
}

func (l *Logger) Critical(v ...any) {
	msg := l.logLevels[syslog.LOG_CRIT] + fmt.Sprint(v...)

	if l.syslog != nil {
		l.syslog.Crit(msg)
		return
	}

	if l.level <= syslog.LOG_CRIT {
		l.logger.Println(msg)
	}
}

func (l *Logger) Error(v ...any) {
	msg := l.logLevels[syslog.LOG_ERR] + fmt.Sprint(v...)

	if l.syslog != nil {
		l.syslog.Err(msg)
		return
	}

	if l.level <= syslog.LOG_ERR {
		l.logger.Println(msg)
	}
}

func (l *Logger) Warning(v ...any) {
	msg := l.logLevels[syslog.LOG_WARNING] + fmt.Sprint(v...)

	if l.syslog != nil {
		l.syslog.Warning(msg)
		return
	}

	if l.level <= syslog.LOG_WARNING {
		l.logger.Println(msg)
	}
}

func (l *Logger) Info(v ...any) {
	msg := l.logLevels[syslog.LOG_INFO] + fmt.Sprint(v...)

	if l.syslog != nil {
		l.syslog.Info(msg)
		return
	}

	if l.level <= syslog.LOG_INFO {
		l.logger.Println(msg)
	}
}

func (l *Logger) Debug(v ...any) {
	msg := l.logLevels[syslog.LOG_DEBUG] + fmt.Sprint(v...)

	if l.syslog != nil {
		l.syslog.Debug(msg)
		return
	}

	if l.level <= syslog.LOG_DEBUG {
		l.logger.Println(msg)
	}
}
