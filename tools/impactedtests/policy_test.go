// Copyright (C) 2019, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectedTargetsRejectsUnsupportedPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(t.Context(), "HEAD..HEAD", "test", []string{"//..."}, "unknown")
	require.EqualError(t, err, `unknown policy "unknown"`)
}

func TestSelectedTargetsRejectsUnsupportedCommandForUnitPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(t.Context(), "HEAD..HEAD", "build", []string{"//..."}, policyUnitGoTest)
	require.EqualError(t, err, `policy "unit-go-test" supports only bazel command "test"`)
}

func TestSelectedTargetsRejectsUnsupportedCommandForAutoPolicy(t *testing.T) {
	t.Parallel()

	_, err := selectedTargets(t.Context(), "HEAD..HEAD", "build", []string{"//..."}, policyAuto)
	require.EqualError(t, err, `policy "auto" does not support bazel command "build"`)
}
