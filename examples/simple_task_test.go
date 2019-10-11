package examples

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/qgymje/gostage"
)

func Test_simpleTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	lg := gostage.NewStdLogger()

	data := []int{1, 2, 3, 4, 5}
	idx := 0
	producer := gostage.WorkHandler(func(_ interface{}) (interface{}, error) {
		if idx == len(data) {
			return nil, gostage.ErrQuit
		}

		i := data[idx]
		idx++

		if idx < 1 {
			log.Println("no data")
			return nil, gostage.ErrNoData
		}

		return i, nil
	})

	consumer := gostage.WorkHandler(func(in interface{}) (interface{}, error) {
		log.Println("got data", in)
		return nil, nil
	})

	config := []*gostage.Config{
		{
			Worker: producer,
		},
		{
			Worker:      consumer,
			SubscribeTo: producer,
		},
	}

	gs := gostage.New(ctx, config, lg, gostage.WithNoDataCount(1), gostage.WithNoDataCountSleep(1*time.Second))
	gs.Run(func() {
		log.Println("I'm done")
		cancel()
	})
}
