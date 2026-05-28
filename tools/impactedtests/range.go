package main

import (
	"fmt"
	"strings"
)

type diffRange struct {
	baseRev            string
	headRev            string
	includeWorkingTree bool
}

func parseDiffRange(input string) (diffRange, error) {
	if strings.Contains(input, "...") || strings.Count(input, "..") != 1 {
		return diffRange{}, fmt.Errorf("must contain exactly one '..' separator")
	}

	parts := strings.SplitN(input, "..", 2)
	baseRev := strings.TrimSpace(parts[0])
	headRev := strings.TrimSpace(parts[1])
	if baseRev == "" {
		return diffRange{}, fmt.Errorf("base revision must not be empty")
	}

	return diffRange{
		baseRev:            baseRev,
		headRev:            headRev,
		includeWorkingTree: headRev == "",
	}, nil
}
