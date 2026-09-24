package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kostis-codefresh/sequencer/pkg/spec"
)

// checkPrerequisites verifies every program required by s.Prerequisites is
// available on PATH, returning a single error listing everything missing.
func checkPrerequisites(s *spec.Spec) error {
	if s.Prerequisites == nil {
		return nil
	}

	var missing []string
	for _, name := range s.Prerequisites.Programs {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required program(s): %s", strings.Join(missing, ", "))
	}

	return nil
}
