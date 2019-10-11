package gostage

import (
	"errors"
	"runtime/debug"
)

// ErrSupervision if restart time is reached will cause this error
var ErrSupervision = errors.New("out of supervision")

type supervisor struct {
	maxRestart   int
	restartCount int
	restartChan  chan struct{}
	errChan      chan error
	workerFunc   func()
	logger       Logger
}

// Supervise supervises a function which is running in a goroutine
// automatically restart it when crashes
func Supervise(workerFunc func(), maxRestart int, logger Logger) chan error {
	s := &supervisor{
		maxRestart:  maxRestart,
		restartChan: make(chan struct{}),
		errChan:     make(chan error),
		workerFunc:  workerFunc,
	}
	go s.monitor()
	return s.errChan
}

func (s *supervisor) monitor() {
	go s.work()

	for range s.restartChan {
		s.restartCount++
		if s.restartCount > s.maxRestart {
			s.errChan <- ErrSupervision
			return
		}
		go s.work()
	}
}

func (s *supervisor) work() {
	defer func() {
		if err := recover(); err != nil {
			s.logger.Error("got recover error: %v\n%s\n", err, string(debug.Stack()))
			s.restartChan <- struct{}{}
		}
	}()
	s.workerFunc()
}
