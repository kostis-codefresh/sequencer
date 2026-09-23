package main

import (
	"fmt"
	"log"
	"os"

	"github.com/kostis-codefresh/sequencer/pkg/engine"
	"github.com/kostis-codefresh/sequencer/pkg/spec"
)

const defaultPipelineFile = "pipeline.yaml"

func main() {
	path, err := pipelinePath(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	s, err := spec.Load(path)
	if err != nil {
		log.Fatal(err)
	}

	if err := s.Validate(); err != nil {
		log.Fatalf("invalid pipeline %s: %v", path, err)
	}

	if err := engine.Run(s); err != nil {
		log.Fatalf("pipeline %s failed: %v", path, err)
	}
}

// pipelinePath returns the pipeline file to load: the single argument in
// args if given, otherwise defaultPipelineFile.
func pipelinePath(args []string) (string, error) {
	switch len(args) {
	case 0:
		return defaultPipelineFile, nil
	case 1:
		return args[0], nil
	default:
		return "", fmt.Errorf("usage: sequencer [pipeline-file]")
	}
}
