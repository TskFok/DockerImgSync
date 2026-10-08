package sync

import (
	"context"
	"errors"
	gosync "sync"
	"testing"
	"time"
)

type finishCall struct {
	id      int32
	trigger string
	result  Result
}

type fakeStore struct {
	mu        gosync.Mutex
	due       []Task
	byID      map[int32]Task
	listCalls int
	marked    []int32
	markCalls int
	finished  []finishCall
	finishCh  chan finishCall
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byID:     make(map[int32]Task),
		finishCh: make(chan finishCall, 8),
	}
}

func sampleTask(id int32, lastDigest, lastStatus string) Task {
	return Task{
		ID:                id,
		SourceImage:       "nginx:latest",
		RegistryAddress:   "registry.example.com",
		RegistryNamespace: "ns",
		DestAuth:          &Auth{Username: "dest-user", Password: "registry-pass"},
		LastDigest:        lastDigest,
		LastStatus:        lastStatus,
	}
}

func (f *fakeStore) ListDue(ctx context.Context, now time.Time) ([]Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	out := make([]Task, len(f.due))
	copy(out, f.due)
	return out, nil
}

func (f *fakeStore) MarkRunning(ctx context.Context, ids []int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls++
	f.marked = append([]int32(nil), ids...)
	return nil
}

func (f *fakeStore) Finish(ctx context.Context, id int32, trigger string, result Result) error {
	call := finishCall{id: id, trigger: trigger, result: result}
	f.mu.Lock()
	f.finished = append(f.finished, call)
	f.mu.Unlock()
	f.finishCh <- call
	return nil
}

func (f *fakeStore) ResetRunning(ctx context.Context, now time.Time) error {
	return nil
}

func (f *fakeStore) Get(ctx context.Context, id int32) (Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.byID[id]
	if !ok {
		return Task{}, errors.New("任务不存在")
	}
	return task, nil
}

func waitFinishes(t *testing.T, ch <-chan finishCall, n int) []finishCall {
	t.Helper()
	got := make([]finishCall, 0, n)
	deadline := time.After(2 * time.Second)
	for len(got) < n {
		select {
		case c := <-ch:
			got = append(got, c)
		case <-deadline:
			t.Fatalf("timeout waiting for %d Finish calls, got %d", n, len(got))
		}
	}
	return got
}

func TestTickMarksDueAndFinishesSkipped(t *testing.T) {
	store := newFakeStore()
	store.due = []Task{
		sampleTask(1, "sha256:same", "idle"),
		sampleTask(2, "sha256:same", "idle"),
	}
	s := NewScheduler(store, &fakeEngine{digest: "sha256:same"})
	if err := s.Tick(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	marked := append([]int32(nil), store.marked...)
	markCalls := store.markCalls
	store.mu.Unlock()
	if markCalls != 1 || len(marked) != 2 || marked[0] != 1 || marked[1] != 2 {
		t.Fatalf("MarkRunning got %v calls=%d", marked, markCalls)
	}
	finishes := waitFinishes(t, store.finishCh, 2)
	for _, f := range finishes {
		if f.trigger != "schedule" || f.result.Status != "skipped" {
			t.Fatalf("finish %+v status %s", f, f.result.Status)
		}
	}
}

func TestTickEmptyDoesNotMark(t *testing.T) {
	store := newFakeStore()
	store.due = []Task{}
	s := NewScheduler(store, &fakeEngine{digest: "sha256:same"})
	if err := s.Tick(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.marked) != 0 || store.markCalls != 0 {
		t.Fatalf("marked %v calls=%d", store.marked, store.markCalls)
	}
}

func TestSyncNowBusy(t *testing.T) {
	store := newFakeStore()
	store.byID[1] = sampleTask(1, "sha256:same", "running")
	s := NewScheduler(store, &fakeEngine{digest: "sha256:same"})
	err := s.SyncNow(context.Background(), 1)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.marked) != 0 {
		t.Fatalf("marked %v", store.marked)
	}
}

func TestSyncNowIdleMarksAndFinishesManual(t *testing.T) {
	store := newFakeStore()
	store.byID[3] = sampleTask(3, "sha256:same", "idle")
	s := NewScheduler(store, &fakeEngine{digest: "sha256:same"})
	if err := s.SyncNow(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	marked := append([]int32(nil), store.marked...)
	markCalls := store.markCalls
	store.mu.Unlock()
	if markCalls != 1 || len(marked) != 1 || marked[0] != 3 {
		t.Fatalf("MarkRunning got %v calls=%d", marked, markCalls)
	}
	finishes := waitFinishes(t, store.finishCh, 1)
	if finishes[0].trigger != "manual" || finishes[0].result.Status != "skipped" {
		t.Fatalf("finish %+v status %s", finishes[0], finishes[0].result.Status)
	}
}

type blockingEngine struct {
	digest  string
	started chan struct{}
	unblock chan struct{}
	once    gosync.Once
}

func (b *blockingEngine) Digest(ctx context.Context, ref string, auth *Auth) (string, error) {
	b.once.Do(func() { close(b.started) })
	<-b.unblock
	return b.digest, nil
}

func (b *blockingEngine) Copy(ctx context.Context, src, dst string, srcAuth, dstAuth *Auth) error {
	return nil
}

func TestTickDoesNotRerunInFlightSyncNow(t *testing.T) {
	task := sampleTask(1, "sha256:same", "idle")
	store := newFakeStore()
	store.byID[1] = task
	store.due = []Task{task}
	eng := &blockingEngine{
		digest:  "sha256:same",
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	s := NewScheduler(store, eng)
	if err := s.SyncNow(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-eng.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Digest did not start")
	}
	if err := s.Tick(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	markCalls := store.markCalls
	store.mu.Unlock()
	if markCalls != 1 {
		t.Fatalf("Tick should not MarkRunning an already claimed id, calls=%d", markCalls)
	}
	close(eng.unblock)
	finishes := waitFinishes(t, store.finishCh, 1)
	if finishes[0].id != 1 {
		t.Fatalf("finish %+v", finishes[0])
	}
	select {
	case extra := <-store.finishCh:
		t.Fatalf("Finish for id %d happened twice: %+v", extra.id, extra)
	case <-time.After(150 * time.Millisecond):
	}
	store.mu.Lock()
	n := len(store.finished)
	store.mu.Unlock()
	if n != 1 {
		t.Fatalf("Finish count %d", n)
	}
}
