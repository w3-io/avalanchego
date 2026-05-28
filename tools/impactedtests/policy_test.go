package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectedTargetsRejectsUnsupportedPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(context.Background(), "HEAD..HEAD", "test", []string{"//..."}, "unknown")
	require.EqualError(t, err, `unknown policy "unknown"`)
}

func TestSelectedTargetsRejectsUnsupportedCommandForUnitPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(context.Background(), "HEAD..HEAD", "build", []string{"//..."}, policyUnitGoTest)
	require.EqualError(t, err, `policy "unit-go-test" supports only bazel command "test"`)
}

func TestSelectedTargetsRejectsUnsupportedCommandForAutoPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(context.Background(), "HEAD..HEAD", "build", []string{"//..."}, policyAuto)
	require.EqualError(t, err, `policy "auto" does not support bazel command "build"`)
}
