// Package spec loads and validates sequencer pipeline files, following the
// format documented in spec/reference.yaml.
package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Retry configures how a failed task should be retried.
type Retry struct {
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
	Count    int    `yaml:"count"`
}

// Prerequisites lists what must already be present before the pipeline runs.
type Prerequisites struct {
	Programs []string `yaml:"programs"`
}

// Task is a single unit of work in the pipeline.
type Task struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Deps        []string `yaml:"deps"`
	Before      []string `yaml:"before"`
	Cmds        []string `yaml:"cmds"`
	Verify      []string `yaml:"verify"`
	After       []string `yaml:"after"`
	Cleanup     []string `yaml:"cleanup"`
	Fail        []string `yaml:"fail"`
}

// SequenceItem wraps a Task to match the `- task: {...}` nesting used in the
// pipeline YAML.
type SequenceItem struct {
	Task Task `yaml:"task"`
}

// Spec is the top-level pipeline definition.
type Spec struct {
	Version       string         `yaml:"version"`
	Environment   string         `yaml:"environment"`
	Purpose       string         `yaml:"purpose"`
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	Prerequisites *Prerequisites `yaml:"prerequisites"`
	Retry         *Retry         `yaml:"retry"`
	Sequence      []SequenceItem `yaml:"sequence"`
}

// Tasks returns the pipeline's tasks in declaration order.
func (s *Spec) Tasks() []Task {
	tasks := make([]Task, len(s.Sequence))
	for i, item := range s.Sequence {
		tasks[i] = item.Task
	}
	return tasks
}

// Load reads and parses a pipeline file at path.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &s, nil
}
