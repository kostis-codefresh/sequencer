package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSpec(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func TestLoad(t *testing.T) {
	path := writeSpec(t, `
name: 'demo'
sequence:
- task:
    name: 'hello'
    cmds:
      - echo "hi"
`)

	s, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, "demo", s.Name)
	require.Len(t, s.Tasks(), 1)
	require.Equal(t, "hello", s.Tasks()[0].Name)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

func TestValidate_OK(t *testing.T) {
	s := &Spec{Sequence: []SequenceItem{
		{Task: Task{Name: "a", Cmds: []string{"echo a"}}},
		{Task: Task{Name: "b", Deps: []string{"a"}, Cmds: []string{"echo b"}}},
	}}
	require.NoError(t, s.Validate())
}

func TestValidate_DuplicateName(t *testing.T) {
	s := &Spec{Sequence: []SequenceItem{
		{Task: Task{Name: "a", Cmds: []string{"echo a"}}},
		{Task: Task{Name: "a", Cmds: []string{"echo a"}}},
	}}
	require.ErrorContains(t, s.Validate(), "duplicate task name")
}

func TestValidate_UnknownDep(t *testing.T) {
	s := &Spec{Sequence: []SequenceItem{
		{Task: Task{Name: "a", Deps: []string{"missing"}, Cmds: []string{"echo a"}}},
	}}
	require.ErrorContains(t, s.Validate(), `unknown task "missing"`)
}

func TestValidate_NoCmds(t *testing.T) {
	s := &Spec{Sequence: []SequenceItem{
		{Task: Task{Name: "a"}},
	}}
	require.ErrorContains(t, s.Validate(), "no cmds")
}

func TestValidate_Cycle(t *testing.T) {
	s := &Spec{Sequence: []SequenceItem{
		{Task: Task{Name: "a", Deps: []string{"b"}, Cmds: []string{"echo a"}}},
		{Task: Task{Name: "b", Deps: []string{"a"}, Cmds: []string{"echo b"}}},
	}}
	require.ErrorContains(t, s.Validate(), "dependency cycle")
}
