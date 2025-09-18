// types.go
package main

import "time"

// Config holds all configuration for the scraper
type Config struct {
	MaxWorkers        int
	MaxSeriesWorkers  int
	MaxChapterWorkers int
	MaxImageWorkers   int
	HTTPTimeout       time.Duration
	MaxRetries        int
	RetryDelay        time.Duration
}

// WorkerPool manages concurrent workers
type WorkerPool struct {
	semaphore chan struct{}
}

func NewWorkerPool(limit int) *WorkerPool {
	return &WorkerPool{
		semaphore: make(chan struct{}, limit),
	}
}

func (p *WorkerPool) Acquire() {
	p.semaphore <- struct{}{}
}

func (p *WorkerPool) Release() {
	<-p.semaphore
}
