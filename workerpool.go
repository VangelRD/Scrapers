// workerpool.go
package main

// WorkerPool manages concurrent workers with a semaphore pattern
type WorkerPool struct {
	semaphore chan struct{}
}

// NewWorkerPool creates a new worker pool with the specified limit
func NewWorkerPool(limit int) *WorkerPool {
	return &WorkerPool{
		semaphore: make(chan struct{}, limit),
	}
}

// Acquire blocks until a worker slot is available
func (p *WorkerPool) Acquire() {
	p.semaphore <- struct{}{}
}

// Release frees up a worker slot
func (p *WorkerPool) Release() {
	<-p.semaphore
}
