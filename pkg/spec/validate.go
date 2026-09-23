package spec

import "fmt"

// Validate checks that the pipeline's task graph is well-formed: task names
// are non-empty and unique, every dependency refers to a declared task,
// every task has at least one command, and the dependency graph has no
// cycles.
func (s *Spec) Validate() error {
	tasks := s.Tasks()

	byName := make(map[string]Task, len(tasks))
	for _, t := range tasks {
		if t.Name == "" {
			return fmt.Errorf("task has no name")
		}
		if _, exists := byName[t.Name]; exists {
			return fmt.Errorf("duplicate task name: %q", t.Name)
		}
		byName[t.Name] = t
	}

	for _, t := range tasks {
		if len(t.Cmds) == 0 {
			return fmt.Errorf("task %q has no cmds", t.Name)
		}
		for _, dep := range t.Deps {
			if _, exists := byName[dep]; !exists {
				return fmt.Errorf("task %q depends on unknown task %q", t.Name, dep)
			}
		}
	}

	return checkCycles(byName)
}

// checkCycles runs a depth-first search over the deps graph, failing on the
// first cycle it finds.
func checkCycles(byName map[string]Task) error {
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(byName))

	var visit func(name string) error
	visit = func(name string) error {
		switch state[name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle detected at task %q", name)
		}

		state[name] = visiting
		for _, dep := range byName[name].Deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[name] = done
		return nil
	}

	for name := range byName {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}
