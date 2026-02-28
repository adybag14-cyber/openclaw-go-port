package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type JobState string

const (
	JobQueued    JobState = "queued"
	JobRunning   JobState = "running"
	JobSucceeded JobState = "succeeded"
	JobFailed    JobState = "failed"
)

type Job struct {
	ID          string         `json:"id"`
	RequestID   string         `json:"request_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Method      string         `json:"method"`
	Params      map[string]any `json:"params,omitempty"`
	State       JobState       `json:"state"`
	Result      any            `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	SubmittedAt time.Time      `json:"submitted_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
}

type Stats struct {
	WorkerCount int `json:"worker_count"`
	QueueDepth  int `json:"queue_depth"`
	Queued      int `json:"queued"`
	Running     int `json:"running"`
	Succeeded   int `json:"succeeded"`
	Failed      int `json:"failed"`
}

type Executor func(context.Context, Job) (any, error)

type Scheduler struct {
	executor Executor
	queue    chan string

	mu   sync.RWMutex
	jobs map[string]*jobRecord

	workerCount int
	seq         atomic.Uint64
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type jobRecord struct {
	job  Job
	done chan struct{}
}

func New(workerCount int, queueSize int, executor Executor) *Scheduler {
	if workerCount < 1 {
		workerCount = 1
	}
	if queueSize < workerCount {
		queueSize = workerCount * 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		executor:    executor,
		queue:       make(chan string, queueSize),
		jobs:        make(map[string]*jobRecord, queueSize),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := 0; i < workerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *Scheduler) Submit(requestID string, sessionID string, method string, params map[string]any) (Job, error) {
	select {
	case <-s.ctx.Done():
		return Job{}, errors.New("scheduler stopped")
	default:
	}

	id := fmt.Sprintf("job-%06d", s.seq.Add(1))
	now := time.Now().UTC()
	rec := &jobRecord{
		job: Job{
			ID:          id,
			RequestID:   requestID,
			SessionID:   sessionID,
			Method:      method,
			Params:      cloneMap(params),
			State:       JobQueued,
			SubmittedAt: now,
		},
		done: make(chan struct{}),
	}

	s.mu.Lock()
	s.jobs[id] = rec
	s.mu.Unlock()

	select {
	case s.queue <- id:
		return rec.job, nil
	default:
		s.mu.Lock()
		delete(s.jobs, id)
		s.mu.Unlock()
		return Job{}, errors.New("scheduler queue full")
	}
}

func (s *Scheduler) Get(id string) (Job, bool) {
	s.mu.RLock()
	rec, ok := s.jobs[id]
	if !ok {
		s.mu.RUnlock()
		return Job{}, false
	}
	job := cloneJob(rec.job)
	s.mu.RUnlock()
	return job, true
}

func (s *Scheduler) Wait(ctx context.Context, id string, timeout time.Duration) (Job, bool) {
	s.mu.RLock()
	rec, ok := s.jobs[id]
	if !ok {
		s.mu.RUnlock()
		return Job{}, false
	}
	doneCh := rec.done
	job := cloneJob(rec.job)
	s.mu.RUnlock()

	if job.State == JobSucceeded || job.State == JobFailed {
		return job, true
	}

	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-doneCh:
	case <-timer.C:
	case <-ctx.Done():
	}

	return s.Get(id)
}

func (s *Scheduler) SnapshotStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := Stats{
		WorkerCount: s.workerCount,
		QueueDepth:  len(s.queue),
	}

	for _, rec := range s.jobs {
		switch rec.job.State {
		case JobQueued:
			stats.Queued++
		case JobRunning:
			stats.Running++
		case JobSucceeded:
			stats.Succeeded++
		case JobFailed:
			stats.Failed++
		}
	}
	return stats
}

func (s *Scheduler) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.queue:
			s.executeJob(id)
		}
	}
}

func (s *Scheduler) executeJob(id string) {
	s.mu.Lock()
	rec, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	rec.job.State = JobRunning
	rec.job.StartedAt = &now
	jobCopy := cloneJob(rec.job)
	s.mu.Unlock()

	result, err := s.executor(s.ctx, jobCopy)

	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok = s.jobs[id]
	if !ok {
		return
	}
	finished := time.Now().UTC()
	rec.job.FinishedAt = &finished
	if err != nil {
		rec.job.State = JobFailed
		rec.job.Error = err.Error()
	} else {
		rec.job.State = JobSucceeded
		rec.job.Result = result
	}
	select {
	case <-rec.done:
	default:
		close(rec.done)
	}
}

func cloneJob(in Job) Job {
	out := in
	out.Params = cloneMap(in.Params)
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
