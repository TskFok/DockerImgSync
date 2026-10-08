package sync

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"
	"time"
)

const PollInterval = 15 * time.Second

var ErrBusy = errors.New("任务正在同步")

type Store interface {
	ListDue(ctx context.Context, now time.Time) ([]Task, error)
	MarkRunning(ctx context.Context, ids []int32) error
	Finish(ctx context.Context, id int32, trigger string, result Result) error
	ResetRunning(ctx context.Context, now time.Time) error
	Get(ctx context.Context, id int32) (Task, error)
}

type Scheduler struct {
	store Store
	eng   Engine
	sem   chan struct{}
	mu    gosync.Mutex
	locks map[int32]*gosync.Mutex
}

func NewScheduler(store Store, eng Engine) *Scheduler {
	return &Scheduler{
		store: store,
		eng:   eng,
		sem:   make(chan struct{}, 2),
		locks: make(map[int32]*gosync.Mutex),
	}
}

func (s *Scheduler) taskLock(id int32) *gosync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[id]; ok {
		return m
	}
	m := &gosync.Mutex{}
	s.locks[id] = m
	return m
}

func (s *Scheduler) Tick(ctx context.Context, now time.Time) error {
	tasks, err := s.store.ListDue(ctx, now)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]int32, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	if err := s.store.MarkRunning(ctx, ids); err != nil {
		return err
	}
	for _, task := range tasks {
		task := task
		go s.run(ctx, task, "schedule")
	}
	return nil
}

func (s *Scheduler) run(ctx context.Context, task Task, trigger string) {
	s.sem <- struct{}{}
	defer func() { <-s.sem }()
	result := Run(ctx, s.eng, task, time.Now)
	if err := s.store.Finish(ctx, task.ID, trigger, result); err != nil {
		fmt.Println(err)
	}
}

func (s *Scheduler) SyncNow(ctx context.Context, id int32) error {
	lock := s.taskLock(id)
	lock.Lock()
	task, err := s.store.Get(ctx, id)
	if err != nil {
		lock.Unlock()
		return err
	}
	if task.LastStatus == "running" {
		lock.Unlock()
		return ErrBusy
	}
	if err := s.store.MarkRunning(ctx, []int32{id}); err != nil {
		lock.Unlock()
		return err
	}
	lock.Unlock()
	go s.run(ctx, task, "manual")
	return nil
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.store.ResetRunning(ctx, time.Now()); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := s.Tick(ctx, now); err != nil {
					fmt.Println(err)
				}
			}
		}
	}()
	return nil
}
