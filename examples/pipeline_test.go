package examples

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/qgymje/gostage"
)

var count int
var mu sync.Mutex

type Producer struct {
}

func (p *Producer) HandleEvent(_ interface{}) (interface{}, error) {
	mu.Lock()
	defer mu.Unlock()
	if count == 10 {
		return nil, gostage.ErrQuit
	}

	count++
	return count, nil
}

type ProducerConsumer struct {
}

func (c *ProducerConsumer) Create() gostage.Worker {
	return &ProducerConsumer{}
}

func (c *ProducerConsumer) HandleEvent(data interface{}) (interface{}, error) {
	log.Printf("producer consumer: %+v", data)
	return data, nil
}

func producerConsumer2(data interface{}) (interface{}, error) {
	log.Printf("producer consumer2: %+v", data)
	return data, nil
}

type Consumer struct {
	count int
}

func (c *Consumer) Create() gostage.Worker {
	return &Consumer{}
}

func (c *Consumer) Close() {
	log.Printf("consumer clean up: count = %d", c.count)
}

func (c *Consumer) HandleEvent(data interface{}) (interface{}, error) {
	c.count++
	log.Printf("consumer: receive = %+v, count = %d", data, c.count)
	time.Sleep(1e9)
	//panic("I'm blowing")
	return nil, nil
}

func Test_Pipeline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	lg := gostage.NewStdLogger()
	p := &Producer{}
	pc := &ProducerConsumer{}
	pc2 := gostage.WorkHandler(producerConsumer2)
	c := &Consumer{}

	configs := []*gostage.Config{
		{
			Worker: p,
		},
		{
			Name:        "consumer",
			Size:        2,
			Restart:     4,
			Worker:      c,
			SubscribeTo: pc2,
		},
		{
			Worker:      pc2,
			SubscribeTo: pc,
		},
		{
			Worker:      pc,
			Size:        2,
			SubscribeTo: p,
		},
	}

	gs := gostage.New(ctx, configs, lg)
	gs.Run(func() {
		cancel()
		log.Println("done!!!")
	})

	/*
		gs.RunAsync(func() {
			cancel()
			log.Println("done!!!")
		})

		time.Sleep(6 * time.Second)
	*/
}
