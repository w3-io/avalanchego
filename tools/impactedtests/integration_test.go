// Copyright (C) 2019, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifestNoChanges(t *testing.T) {
	repoRoot := repoRootForTest(t)
	toolPath := buildToolForTest(t, repoRoot)
	worktree := addWorktreeForTest(t, repoRoot)

	output := runToolForTest(t, worktree, toolPath, "manifest", "--range", "HEAD..HEAD", "--scope", "//...", "--scope", "-//graft/...")
	require.Empty(t, output)
}

func TestManifestChangedTestFileOnly(t *testing.T) {
	repoRoot := repoRootForTest(t)
	toolPath := buildToolForTest(t, repoRoot)
	worktree := addWorktreeForTest(t, repoRoot)

	appendLineForTest(t, filepath.Join(worktree, "utils/set/set_test.go"), "// impactedtests test-only integration change")

	output := runToolForTest(t, worktree, toolPath, "manifest", "--range", "HEAD..", "--scope", "//...", "--scope", "-//graft/...")
	require.Equal(t, []string{"//utils/set:set_test"}, splitLines(output))
}

func TestManifestChangedSharedUtilityLibrary(t *testing.T) {
	repoRoot := repoRootForTest(t)
	toolPath := buildToolForTest(t, repoRoot)
	worktree := addWorktreeForTest(t, repoRoot)

	appendLineForTest(t, filepath.Join(worktree, "utils/atomic.go"), "// impactedtests shared-library integration change")

	output := runToolForTest(t, worktree, toolPath, "manifest", "--range", "HEAD..", "--scope", "//...", "--scope", "-//graft/...")
	labels := splitLines(output)
	require.Contains(t, labels, "//utils:utils_test")
	require.Contains(t, labels, "//api/admin:admin_test")
	require.Greater(t, len(labels), 1)
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(runInDirForTest(t, "", "git", "rev-parse", "--show-toplevel"))
}

func buildToolForTest(t *testing.T, repoRoot string) string {
	t.Helper()
	binaryDir := t.TempDir()
	binaryPath := filepath.Join(binaryDir, "impactedtests")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./tools/impactedtests")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, execError("build impactedtests binary", err, output))
	return binaryPath
}

func addWorktreeForTest(t *testing.T, repoRoot string) string {
	t.Helper()
	worktree := t.TempDir()
	runInDirForTest(t, repoRoot, "git", "worktree", "add", "--detach", worktree, "HEAD")
	t.Cleanup(func() {
		runInDirForTest(t, repoRoot, "git", "worktree", "remove", "--force", worktree)
	})
	return worktree
}

func appendLineForTest(t *testing.T, path string, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	defer file.Close()
	_, err = file.WriteString("\n" + line + "\n")
	require.NoError(t, err)
}

func runToolForTest(t *testing.T, dir string, toolPath string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(runInDirForTest(t, dir, toolPath, args...))
}

func runInDirForTest(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, execError(name, err, output))
	return string(output)
}

func execError(action string, err error, output []byte) error {
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%s: %w: %s", action, err, trimmed)
}
