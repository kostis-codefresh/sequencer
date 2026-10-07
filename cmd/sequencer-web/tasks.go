package main

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

type task struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	User      string    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// taskStore holds tasks in memory only; they are lost on restart.
type taskStore struct {
	mu     sync.RWMutex
	tasks  map[int]task
	nextID int
}

func newTaskStore() *taskStore {
	return &taskStore{tasks: map[int]task{}, nextID: 1}
}

func (s *taskStore) add(name, user string) (task, error) {
	name, user = strings.TrimSpace(name), strings.TrimSpace(user)
	if name == "" || user == "" {
		return task{}, errors.New("name and user are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	t := task{ID: s.nextID, Name: name, User: user, CreatedAt: time.Now().UTC()}
	s.tasks[t.ID] = t
	s.nextID++
	return t, nil
}

// remove deletes the task and reports whether it existed.
func (s *taskStore) remove(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.tasks[id]
	delete(s.tasks, id)
	return ok
}

// list returns all tasks, oldest first.
func (s *taskStore) list() []task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	slices.SortFunc(tasks, func(a, b task) int { return cmp.Compare(a.ID, b.ID) })
	return tasks
}
