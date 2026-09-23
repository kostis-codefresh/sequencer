package main

import (
	"testing"

	"github.com/kostis-codefresh/sequencer/pkg/engine"
	"github.com/kostis-codefresh/sequencer/pkg/spec"
	"github.com/stretchr/testify/require"
)

func TestHelloWorldPipeline(t *testing.T) {
	s, err := spec.Load("examples/simple-hello-world/pipeline.yaml")
	require.NoError(t, err)
	require.NoError(t, s.Validate())
	require.NoError(t, engine.Run(s))
}
