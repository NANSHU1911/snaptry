package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"snaptry-gateway/provider"
)

// Task lifecycle: queued -> running -> done | failed
type Task struct {
	ID             string `json:"task_id"`
	Status         string `json:"status"`
	ResultURL      string `json:"result_url,omitempty"`
	Error          string `json:"error,omitempty"`
	Provider       string `json:"-"`
	ProviderTaskID string `json:"-"`
	CreatedAt      int64  `json:"created_at"`
}

type Store struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	prov     provider.Provider
	cache    *Cache
	pollWait time.Duration
	pollMax  time.Duration
}

func NewStore(prov provider.Provider, cache *Cache) *Store {
	return &Store{
		tasks:    make(map[string]*Task),
		prov:     prov,
		cache:    cache,
		pollWait: 4 * time.Second,
		pollMax:  6 * time.Minute,
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Submit creates a task, checks the result cache first, and otherwise starts
// a background worker that drives the provider to completion.
func (s *Store) Submit(req provider.TryOnRequest) *Task {
	t := &Task{ID: newID(), Status: "queued", Provider: s.prov.Name(), CreatedAt: time.Now().Unix()}

	if url, ok := s.cache.Get(cacheKey(req, s.prov.Name())); ok {
		t.Status = "done"
		t.ResultURL = url
		s.mu.Lock()
		s.tasks[t.ID] = t
		s.mu.Unlock()
		log.Printf("task %s cache HIT provider=%s", t.ID, t.Provider)
		return t
	}

	s.mu.Lock()
	s.tasks[t.ID] = t
	s.mu.Unlock()
	go s.run(t, req)
	return t
}

func (s *Store) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	return t, ok
}

func (s *Store) setStatus(t *Task, status, resultURL, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Status = status
	t.ResultURL = resultURL
	t.Error = errMsg
}

func (s *Store) run(t *Task, req provider.TryOnRequest) {
	s.setStatus(t, "running", "", "")
	log.Printf("task %s submit provider=%s", t.ID, t.Provider)

	ctx, cancel := context.WithTimeout(context.Background(), s.pollMax)
	defer cancel()

	provID, err := s.prov.Submit(ctx, req)
	if err != nil {
		log.Printf("task %s submit failed: %v", t.ID, err)
		s.setStatus(t, "failed", "", "上游提交失败: "+err.Error())
		return
	}
	t.ProviderTaskID = provID

	ticker := time.NewTicker(s.pollWait)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.setStatus(t, "failed", "", "等待上游超时，请稍后重试")
			return
		case <-ticker.C:
			done, url, err := s.prov.Poll(ctx, provID)
			if err != nil {
				log.Printf("task %s poll error: %v", t.ID, err)
				s.setStatus(t, "failed", "", "上游处理失败: "+err.Error())
				return
			}
			if done {
				log.Printf("task %s done provider=%s", t.ID, t.Provider)
				s.cache.Set(cacheKey(req, s.prov.Name()), url)
				s.setStatus(t, "done", url, "")
				return
			}
		}
	}
}
