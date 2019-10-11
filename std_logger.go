package gostage

import (
	"log"
)

type StdLogger struct {
}

func NewStdLogger() *StdLogger {
	log.SetFlags(log.Ltime | log.Lshortfile)
	return &StdLogger{}
}

func (l *StdLogger) Fatal(format string, args ...interface{}) {
	log.Printf("[Fatal]"+format, args...)
}

func (l *StdLogger) Debug(format string, args ...interface{}) {
	log.Printf("[Debug]"+format, args...)
}

func (l *StdLogger) Info(format string, args ...interface{}) {
	log.Printf("[Info]"+format, args...)
}

func (l *StdLogger) Error(format string, args ...interface{}) {
	log.Printf("[Error]"+format, args...)
}
