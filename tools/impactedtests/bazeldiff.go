package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	bazelDiffURL  = "https://github.com/Tinder/bazel-diff/releases/download/v22.0.0/bazel-diff_deploy.jar"
	bazelDiffHash = "sha256-F7opo1MmvosIVObhv29FA2gq9bBBZfeaPn+96apq5uY="
)

type prefetchedFile struct {
	Hash      string `json:"hash"`
	StorePath string `json:"storePath"`
}

func impactedLabels(ctx context.Context, rangeArg string) ([]string, error) {
	repoRoot, err := gitOutput(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	diff, err := parseDiffRange(rangeArg)
	if err != nil {
		return nil, fmt.Errorf("parse range: %w", err)
	}

	if err := ensureRevisionAvailable(ctx, repoRoot, diff.baseRev); err != nil {
		return nil, err
	}
	if diff.headRev != "" {
		if err := ensureRevisionAvailable(ctx, repoRoot, diff.headRev); err != nil {
			return nil, err
		}
	}

	jarPath, err := prefetchBazelDiff(ctx)
	if err != nil {
		return nil, err
	}

	scratchDir, err := os.MkdirTemp("", "impactedtests-")
	if err != nil {
		return nil, fmt.Errorf("create scratch dir: %w", err)
	}
	defer os.RemoveAll(scratchDir)

	bazelPath, err := exec.LookPath("bazel")
	if err != nil {
		return nil, fmt.Errorf("find bazel: %w", err)
	}

	basePath := filepath.Join(scratchDir, "base")
	if err := runGit(ctx, repoRoot, "worktree", "add", "--detach", basePath, diff.baseRev); err != nil {
		return nil, fmt.Errorf("create base worktree: %w", err)
	}
	defer runGit(ctx, repoRoot, "worktree", "remove", "--force", basePath)

	headPath := repoRoot
	if !diff.includeWorkingTree {
		headPath = filepath.Join(scratchDir, "head")
		if err := runGit(ctx, repoRoot, "worktree", "add", "--detach", headPath, diff.headRev); err != nil {
			return nil, fmt.Errorf("create head worktree: %w", err)
		}
		defer runGit(ctx, repoRoot, "worktree", "remove", "--force", headPath)
	}

	baseHashes := filepath.Join(scratchDir, "base-hashes.json")
	headHashes := filepath.Join(scratchDir, "head-hashes.json")
	impactedTargets := filepath.Join(scratchDir, "impacted-targets.txt")

	if err := runBazelDiff(ctx, jarPath, "generate-hashes", "-w", basePath, "-b", bazelPath, baseHashes); err != nil {
		return nil, fmt.Errorf("generate base hashes: %w", err)
	}
	if err := runBazelDiff(ctx, jarPath, "generate-hashes", "-w", headPath, "-b", bazelPath, headHashes); err != nil {
		return nil, fmt.Errorf("generate head hashes: %w", err)
	}
	if err := runBazelDiff(ctx, jarPath, "get-impacted-targets", "-w", headPath, "-b", bazelPath, "-sh", baseHashes, "-fh", headHashes, "-o", impactedTargets); err != nil {
		return nil, fmt.Errorf("compute impacted targets: %w", err)
	}

	return readLines(impactedTargets)
}

func impactedManifest(ctx context.Context, rangeArg string, scopes []string) ([]string, error) {
	return impactedGoTestManifest(ctx, rangeArg, scopes)
}

func queryDirectoryForRange(ctx context.Context, rangeArg string) (string, func(), error) {
	diff, err := parseDiffRange(rangeArg)
	if err != nil {
		return "", nil, fmt.Errorf("parse range: %w", err)
	}
	repoRoot, err := gitOutput(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, fmt.Errorf("resolve repo root: %w", err)
	}

	queryDir := repoRoot
	cleanup := func() {}
	if !diff.includeWorkingTree {
		scratchDir, err := os.MkdirTemp("", "impactedtests-query-")
		if err != nil {
			return "", nil, fmt.Errorf("create query scratch dir: %w", err)
		}

		queryDir = filepath.Join(scratchDir, "head")
		if err := runGit(ctx, repoRoot, "worktree", "add", "--detach", queryDir, diff.headRev); err != nil {
			os.RemoveAll(scratchDir)
			return "", nil, fmt.Errorf("create query worktree: %w", err)
		}
		cleanup = func() {
			_ = runGit(ctx, repoRoot, "worktree", "remove", "--force", queryDir)
			_ = os.RemoveAll(scratchDir)
		}
	}
	return queryDir, cleanup, nil
}

func queryScopedGoTests(ctx context.Context, dir string, scopes []string) ([]string, error) {
	scopeExpr, err := scopeExpression(scopes)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`kind("go_test rule", %s) except attr("tags", "manual", kind("go_test rule", %s))`, scopeExpr, scopeExpr)
	output, err := commandOutput(ctx, dir, "bazel", "query", query)
	if err != nil {
		return nil, fmt.Errorf("query non-manual go_test targets for %q: %w", scopeExpr, err)
	}
	return splitLines(output), nil
}

func prefetchBazelDiff(ctx context.Context) (string, error) {
	output, err := commandOutput(ctx, "", "nix", "store", "prefetch-file", "--json", bazelDiffURL)
	if err != nil {
		return "", fmt.Errorf("prefetch bazel-diff: %w", err)
	}

	var prefetched prefetchedFile
	if err := json.Unmarshal([]byte(output), &prefetched); err != nil {
		return "", fmt.Errorf("parse bazel-diff prefetch metadata: %w", err)
	}
	if prefetched.Hash != bazelDiffHash {
		return "", fmt.Errorf("unexpected bazel-diff hash %q", prefetched.Hash)
	}
	if prefetched.StorePath == "" {
		return "", fmt.Errorf("missing bazel-diff store path")
	}
	return prefetched.StorePath, nil
}

func runBazelDiff(ctx context.Context, jarPath string, args ...string) error {
	if javaPath, err := exec.LookPath("java"); err == nil {
		_, err = commandOutput(ctx, "", javaPath, append([]string{"-jar", jarPath}, args...)...)
		return err
	}

	_, err := commandOutput(ctx, "", "nix", append([]string{"run", "nixpkgs#jdk_headless", "--", "-jar", jarPath}, args...)...)
	return err
}

func ensureRevisionAvailable(ctx context.Context, repoRoot string, rev string) error {
	if err := runGit(ctx, repoRoot, "rev-parse", "--verify", rev+"^{commit}"); err == nil {
		return nil
	}
	if !isLikelySHA(rev) {
		return fmt.Errorf("revision %q is not available locally", rev)
	}
	if err := runGit(ctx, repoRoot, "fetch", "--no-tags", "--depth=1", "origin", rev); err != nil {
		return fmt.Errorf("fetch revision %q: %w", rev, err)
	}
	if err := runGit(ctx, repoRoot, "rev-parse", "--verify", rev+"^{commit}"); err != nil {
		return fmt.Errorf("verify fetched revision %q: %w", rev, err)
	}
	return nil
}

func isLikelySHA(rev string) bool {
	if len(rev) < 7 || len(rev) > 40 {
		return false
	}
	for _, r := range rev {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func gitOutput(ctx context.Context, args ...string) (string, error) {
	return commandOutput(ctx, "", "git", args...)
}

func runGit(ctx context.Context, dir string, args ...string) error {
	_, err := commandOutput(ctx, dir, "git", args...)
	return err
}

func commandOutput(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, trimmed)
	}
	return strings.TrimSpace(string(output)), nil
}

func readLines(path string) ([]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return splitLines(string(bytes)), nil
}

func splitLines(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}
