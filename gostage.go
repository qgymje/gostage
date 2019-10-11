package gostage

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"
)

// ErrNoData if the producer worker generates no data
var ErrNoData = errors.New("no data")

// ErrQuit if producer is about the quit, return this error
var ErrQuit = errors.New("quit")

// DefaultRestart the default restart times for each worker
var DefaultRestart = 1

// DefaultSize the default worker size, default is 1
// gostage.DefaultSize = 100
// will make all workers have 100 goroutines running
var DefaultSize = 1

// NoDataCount count producer returns ErrNoData
var NoDataCount = 100

// NoDataCountSleep if ErrNoData accumulates NoDataCount then sleep
var NoDataCountSleep = time.Second

// Logger the logger interface
type Logger interface {
	Fatal(format string, args ...interface{})
	Error(format string, args ...interface{})
	Info(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// WorkHandler is a handy function type that implements Worker
type WorkHandler func(interface{}) (interface{}, error)

// HandleEvent implements the Worker
func (wh WorkHandler) HandleEvent(in interface{}) (interface{}, error) {
	return wh(in)
}

// Worker describes a worker abstaction
type Worker interface {
	// Create creates a fresh new worker in order to avoid data race
	// optional if worker doesn't  provider this functions, then the size
	// can't be more than 1
	//Create() Worker

	// HandleEvent is called by the framework automatically
	// If a worker acts as the producer, the arguments is nill
	// If a worker acts as the last consumer, the return value is nil
	HandleEvent(interface{}) (interface{}, error)

	// Close clean up some resources
	// optional, if exists then will call this function when worker is quit
	// Close()
}

// Config is a description of a Worker
type Config struct {
	// the worker's name, shows in logger
	Name string
	// the number of worker that will create
	// each worker runs in a goroutine
	Size int
	// the worker itself
	Worker Worker
	// this worker's HandleEvent function will return data which
	// pass to the current worker's HandleEvent's input
	SubscribeTo Worker
	// each worker has a change to restart
	Restart int
}

type linkedWorker struct {
	*Config
	in  chan interface{}
	out chan interface{}
}

// GoStage provides a simple way to run a data pipeline, just like unix pipeline.
type GoStage struct {
	ctx           context.Context
	logger        Logger
	configs       []*Config
	linkedWorkers []*linkedWorker
	errChan       chan error
	quitChan      chan error
	// close goroutines one by one
	stopChan []chan chan struct{}

	noDataCount      int
	noDataCountSleep time.Duration
}

type Option func(gs *GoStage)

func WithNoDataCount(n int) func(*GoStage) {
	return func(gs *GoStage) {
		gs.noDataCount = n
	}
}

func WithNoDataCountSleep(n time.Duration) func(*GoStage) {
	return func(gs *GoStage) {
		gs.noDataCountSleep = n
	}
}

// New creates a new GoStage
func New(ctx context.Context, configs []*Config, logger Logger, opts ...Option) *GoStage {
	gs := &GoStage{
		ctx:           ctx,
		logger:        logger,
		configs:       configs,
		errChan:       make(chan error),
		quitChan:      make(chan error),
		stopChan:      []chan chan struct{}{},
		linkedWorkers: make([]*linkedWorker, 0, len(configs)),
	}

	// set default value
	gs.noDataCount = NoDataCount
	gs.noDataCountSleep = NoDataCountSleep

	for _, opt := range opts {
		opt(gs)
	}

	return gs
}

// Run blocks the current goroutine
func (s *GoStage) Run(fn func()) {
	s.run()

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-s.ctx.Done():
	case <-stopSignals:
	case err := <-s.quitChan:
		s.logger.Error("gostage quit: %+v", err)
	case err := <-s.errChan:
		s.logger.Fatal("gostage fatal error happened: %+v", err)
	}

	s.ensureAllWorkerStopped()
	fn()
}

// RunAsync doesn't block the current goroutine
func (s *GoStage) RunAsync(fn func()) {
	s.run()

	go func() {
		select {
		case <-s.ctx.Done():
		case err := <-s.quitChan:
			s.logger.Error("gostage quit: %+v", err)
		case err := <-s.errChan:
			s.logger.Fatal("gostage fatal error happened: %+v", err)
		}

		s.ensureAllWorkerStopped()
		fn()
	}()
}

func (s *GoStage) ensureAllWorkerStopped() {
	confirm := 0
	for _, stop := range s.stopChan {
		done := make(chan struct{})
		stop <- done
		for range done {
			confirm++
			if confirm == len(s.stopChan) {
				return
			}
		}
	}
}

func (s *GoStage) run() {
	s.buildLinkedWorkers()
	s.setupChannels()
	s.startWorkers()
}

func (s *GoStage) startWorkers() {
	for i := 0; i < len(s.linkedWorkers); i++ {
		size := DefaultSize
		if s.linkedWorkers[i].Size > 0 {
			size = s.linkedWorkers[i].Size
		}

		for n := 0; n < size; n++ {

			i, n := i, n
			w := s.linkedWorkers[i].Worker

			if n != 0 {
				w = s.callWorkerCreate(w)
			}

			stop := make(chan chan struct{})
			s.stopChan = append(s.stopChan, stop)

			restart := DefaultRestart
			if s.linkedWorkers[i].Restart > 0 {
				restart = s.linkedWorkers[i].Restart
			}

			s.errChan = Supervise((func() {
				s.runWorker(w, stop, i, n)
			}), restart, s.logger)
		}
	}
}

func (s *GoStage) callWorkerCreate(w Worker) Worker {
	v := reflect.ValueOf(w)

	if !v.MethodByName("Create").IsValid() {
		panic("worker need Create method")
	}

	ws := v.MethodByName("Create").Call(nil)
	wrk, ok := ws[0].Interface().(Worker)
	if !ok {
		panic("Create function should return a Worker")
	}
	return wrk
}

func (s *GoStage) callWorkerClose(w Worker) {
	v := reflect.ValueOf(w)
	if v.MethodByName("Close").IsValid() {
		v.MethodByName("Close").Call(nil)
	}
}

func (s *GoStage) runWorker(w Worker, stop chan chan struct{}, i, n int) {
	var errNoDataCount int
	if i == 0 {
		for {
			select {
			case done := <-stop:
				s.callWorkerClose(w)
				done <- struct{}{}
				close(done)
				return
			default:
				output, err := w.HandleEvent(nil)
				if err != nil {
					if err == ErrNoData {
						errNoDataCount++
						if errNoDataCount >= s.noDataCount {
							time.Sleep(s.noDataCountSleep)
							errNoDataCount = 0
						}
					} else if err == ErrQuit {
						s.quitChan <- err
						select {
						case done := <-stop:
							s.callWorkerClose(w)
							done <- struct{}{}
							close(done)
							return
						}
					} else {
						s.logger.Error("%s_#%d error: %+v", s.linkedWorkers[i].Name, n, err)
					}
				} else {
					s.linkedWorkers[i].out <- output
				}
			}
		}
	} else if i == len(s.linkedWorkers)-1 {
		for {
			select {
			case done := <-stop:
				s.callWorkerClose(w)
				done <- struct{}{}
				close(done)
				return
			case input := <-s.linkedWorkers[i].in:
				_, err := w.HandleEvent(input)
				if err != nil {
					s.logger.Error("%s_#%d error: %+v, input = %+v", s.linkedWorkers[i].Name, n, err, input)
				}
			}
		}
	} else {
		for {
			select {
			case done := <-stop:
				s.callWorkerClose(w)
				done <- struct{}{}
				close(done)
				return
			case input := <-s.linkedWorkers[i].in:
				output, err := w.HandleEvent(input)
				if err != nil {
					s.logger.Error("%s_#%d error: %+v, input = %+v", s.linkedWorkers[i].Name, n, err, input)
				}
				s.linkedWorkers[i].out <- output
			}
		}
	}
}

func (s *GoStage) buildLinkedWorkers() {
	for config := s.findRoot(); config != nil; config = s.findNext(config) {
		s.setWorkerName(config)
		lw := &linkedWorker{Config: config}
		s.linkedWorkers = append(s.linkedWorkers, lw)
	}
}

func (s *GoStage) setWorkerName(c *Config) {
	if c.Name == "" {
		c.Name = reflect.ValueOf(c).Elem().FieldByName("Worker").Elem().String()
	}
}

func (s *GoStage) setupChannels() {
	for i := 0; i < len(s.linkedWorkers); i++ {
		if i == 0 {
			out := make(chan interface{})
			s.linkedWorkers[i].out = out
		} else if i == len(s.linkedWorkers)-1 {
			s.linkedWorkers[i].in = s.linkedWorkers[i-1].out
		} else {
			out := make(chan interface{})
			s.linkedWorkers[i].in = s.linkedWorkers[i-1].out
			s.linkedWorkers[i].out = out
		}
	}
}

func (s *GoStage) findRoot() *Config {
	for _, config := range s.configs {
		if config.SubscribeTo == nil {
			return config
		}
	}
	return nil
}

func (s *GoStage) findNext(config *Config) *Config {
	wrkVal := reflect.ValueOf(config.Worker)
	for _, c := range s.configs {
		subVal := reflect.ValueOf(c.SubscribeTo)
		if reflect.DeepEqual(subVal, wrkVal) {
			return c
		}
	}
	return nil
}
