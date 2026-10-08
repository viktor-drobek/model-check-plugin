package explore

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// The goroutine pool of the parallel search. Worker 0 is the calling
// goroutine; workers 1 to W-1 are goroutines started once per run, which wait
// for a function per phase on a channel of their own and report their arrival
// at the barrier of the phase (a WaitGroup) when it returns. With one worker no
// goroutine is started at all.
//
// A panic in a worker is an engine defect (a model error is an error return,
// never a panic). It is recovered where it happens, and the recovery counts as
// the worker's arrival at the barrier, so the other workers and the
// coordinator are never left waiting; the coordinator then ends the run with an
// InternalError. The run has no verdict, as a failure of the sequential search
// would have none.

type parPool struct {
	jobs    []chan func(*parWorker)
	barrier sync.WaitGroup // arrivals of the current phase
	exit    sync.WaitGroup // helper goroutines still running
	mu      sync.Mutex
	failure string // the first recovered panic
}

// startPool starts the helper goroutines of workers 1 to W-1.
func (r *parRun) startPool() {
	r.pool = &parPool{}
	for _, w := range r.workers[1:] {
		ch := make(chan func(*parWorker))
		r.pool.jobs = append(r.pool.jobs, ch)
		r.pool.exit.Add(1)
		go func(w *parWorker, ch chan func(*parWorker)) {
			defer r.pool.exit.Done()
			for fn := range ch {
				r.runJob(w, fn, true)
			}
		}(w, ch)
	}
}

// closePool ends the helper goroutines and waits for them.
func (r *parRun) closePool() {
	if r.pool == nil {
		return
	}
	for _, ch := range r.pool.jobs {
		close(ch)
	}
	r.pool.exit.Wait()
}

// runJob runs fn as worker w. A panic is recovered and recorded; barrier is
// whether the job is one of a phase the coordinator waits for.
func (r *parRun) runJob(w *parWorker, fn func(*parWorker), barrier bool) {
	if barrier {
		defer r.pool.barrier.Done()
	}
	defer func() {
		if x := recover(); x != nil {
			r.pool.mu.Lock()
			if r.pool.failure == "" {
				r.pool.failure = fmt.Sprintf("a worker panicked: %v\n%s", x, debug.Stack())
			}
			r.pool.mu.Unlock()
		}
	}()
	fn(w)
}

// phase runs fn on every worker and waits for all of them. It returns an
// InternalError if a worker panicked.
func (r *parRun) phase(fn func(*parWorker)) error {
	if n := len(r.pool.jobs); n > 0 {
		r.pool.barrier.Add(n)
		for _, ch := range r.pool.jobs {
			ch <- fn
		}
	}
	r.runJob(r.workers[0], fn, false)
	r.pool.barrier.Wait()
	r.pool.mu.Lock()
	defer r.pool.mu.Unlock()
	if r.pool.failure != "" {
		return &InternalError{Msg: r.pool.failure}
	}
	return nil
}
