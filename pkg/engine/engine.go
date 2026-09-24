// Package engine executes a validated pipeline spec: tasks run in
// dependency order, with independent tasks running concurrently.
package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/kostis-codefresh/sequencer/pkg/spec"
)

// Run executes every task in s.Sequence, respecting each task's deps.
// Tasks whose deps are already satisfied run concurrently. Execution stops
// scheduling new tasks as soon as any task fails, and Run returns a
// combined error describing every task that failed in that batch.
//
// dir is the directory containing the pipeline file; every cmd runs with
// dir as its working directory, so relative paths in cmds (scripts,
// clones, archives) resolve against the pipeline file's location rather
// than wherever the CLI happened to be invoked from.
func Run(s *spec.Spec, dir string) error {
	if err := checkPrerequisites(s); err != nil {
		return err
	}

	tasks := s.Tasks()

	byName := make(map[string]spec.Task, len(tasks))
	for _, t := range tasks {
		byName[t.Name] = t
	}

	done := make(map[string]bool, len(tasks))

	for len(done) < len(tasks) {
		ready := readyTasks(byName, done)
		if len(ready) == 0 {
			return fmt.Errorf("no runnable tasks remain, but %d task(s) did not complete", len(tasks)-len(done))
		}

		if err := runBatch(ready, dir); err != nil {
			return err
		}

		for _, t := range ready {
			done[t.Name] = true
		}
	}

	return nil
}

// readyTasks returns the not-yet-run tasks whose deps are all satisfied.
func readyTasks(byName map[string]spec.Task, done map[string]bool) []spec.Task {
	var ready []spec.Task
	for name, t := range byName {
		if done[name] {
			continue
		}
		if allSatisfied(t.Deps, done) {
			ready = append(ready, t)
		}
	}
	return ready
}

func allSatisfied(deps []string, done map[string]bool) bool {
	for _, dep := range deps {
		if !done[dep] {
			return false
		}
	}
	return true
}

// runBatch runs every task in tasks concurrently and waits for all of them
// to finish, returning a combined error if any failed.
func runBatch(tasks []spec.Task, dir string) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, t := range tasks {
		wg.Add(1)
		go func(t spec.Task) {
			defer wg.Done()
			if err := runTask(t, dir); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(t)
	}

	wg.Wait()
	return errors.Join(errs...)
}

// runTask runs each of t.Cmds in order, stopping at the first failure.
func runTask(t spec.Task, dir string) error {
	for _, cmd := range t.Cmds {
		c := exec.Command("sh", "-c", cmd)
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("task %q: command %q: %w", t.Name, cmd, err)
		}
	}
	return nil
}
