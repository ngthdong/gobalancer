package limiter

import "context"

type Semaphore chan struct{}

func NewSemaphore(n int) Semaphore {
	return make(chan struct{}, n)
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (s Semaphore) Acquire(ctx context.Context) error {
	select {
	case s <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s Semaphore) Release() {
	<-s
}

func (s Semaphore) Available() int {
	return cap(s) - len(s)
}

func (s Semaphore) InUse() int {
	return len(s)
}
