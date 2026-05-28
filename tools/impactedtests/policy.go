package main

import (
	"context"
	"fmt"
)

const (
	policyAuto       = "auto"
	policyUnitGoTest = "unit-go-test"
)

func selectedTargets(ctx context.Context, rangeArg string, command string, scopes []string, policy string) ([]string, error) {
	if policy == "" {
		policy = policyAuto
	}

	switch policy {
	case policyAuto:
		switch command {
		case "test":
			return impactedGoTestManifest(ctx, rangeArg, scopes)
		default:
			return nil, fmt.Errorf("policy %q does not support bazel command %q", policy, command)
		}
	case policyUnitGoTest:
		if command != "test" {
			return nil, fmt.Errorf("policy %q supports only bazel command %q", policy, "test")
		}
		return impactedGoTestManifest(ctx, rangeArg, scopes)
	default:
		return nil, fmt.Errorf("unknown policy %q", policy)
	}
}

func impactedGoTestManifest(ctx context.Context, rangeArg string, scopes []string) ([]string, error) {
	labels, err := impactedLabels(ctx, rangeArg)
	if err != nil {
		return nil, err
	}

	queryDir, cleanup, err := queryDirectoryForRange(ctx, rangeArg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	testTargets, err := queryScopedGoTests(ctx, queryDir, scopes)
	if err != nil {
		return nil, err
	}
	return filterManifest(labels, testTargets), nil
}
