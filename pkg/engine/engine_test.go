package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kostis-codefresh/sequencer/pkg/spec"
	"github.com/stretchr/testify/require"
)

func TestRun_Success(t *testing.T) {
	s := &spec.Spec{Sequence: []spec.SequenceItem{
		{Task: spec.Task{Name: "left", Cmds: []string{"true"}}},
		{Task: spec.Task{Name: "right", Cmds: []string{"true"}}},
		{Task: spec.Task{Name: "join", Deps: []string{"left", "right"}, Cmds: []string{"true"}}},
	}}

	require.NoError(t, Run(s))
}

func TestRun_TaskFailurePropagates(t *testing.T) {
	s := &spec.Spec{Sequence: []spec.SequenceItem{
		{Task: spec.Task{Name: "boom", Cmds: []string{"false"}}},
	}}

	require.ErrorContains(t, Run(s), "boom")
}

func TestRun_MissingPrerequisiteFailsFast(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	s := &spec.Spec{
		Prerequisites: &spec.Prerequisites{Programs: []string{"definitely-not-a-real-binary-xyz"}},
		Sequence: []spec.SequenceItem{
			{Task: spec.Task{Name: "touch", Cmds: []string{"touch " + marker}}},
		},
	}

	require.ErrorContains(t, Run(s), "definitely-not-a-real-binary-xyz")
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "task ran despite missing prerequisite")
}

func TestRun_PrerequisitePresentSucceeds(t *testing.T) {
	s := &spec.Spec{
		Prerequisites: &spec.Prerequisites{Programs: []string{"sh"}},
		Sequence: []spec.SequenceItem{
			{Task: spec.Task{Name: "left", Cmds: []string{"true"}}},
		},
	}

	require.NoError(t, Run(s))
}

func TestRun_StopsAfterFailedBatch(t *testing.T) {
	s := &spec.Spec{Sequence: []spec.SequenceItem{
		{Task: spec.Task{Name: "boom", Cmds: []string{"false"}}},
		{Task: spec.Task{Name: "after", Deps: []string{"boom"}, Cmds: []string{"true"}}},
	}}

	err := Run(s)
	require.Error(t, err)
}
