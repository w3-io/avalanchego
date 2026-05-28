package main

import "fmt"

var partitions = map[string]string{
	"main":       "//... except //graft/...",
	"coreth":     "//graft/coreth/... union //graft/evm/...",
	"subnet-evm": "//graft/subnet-evm/...",
}

func partitionDefinition(name string) (string, error) {
	definition, ok := partitions[name]
	if !ok {
		return "", fmt.Errorf("unknown partition %q", name)
	}
	return definition, nil
}

func filterManifest(impacted []string, partitionTests []string) []string {
	partitionSet := make(map[string]struct{}, len(partitionTests))
	for _, label := range partitionTests {
		partitionSet[label] = struct{}{}
	}

	filtered := make([]string, 0, len(impacted))
	for _, label := range impacted {
		if _, ok := partitionSet[label]; ok {
			filtered = append(filtered, label)
		}
	}
	return filtered
}

func formatManifest(labels []string) string {
	if len(labels) == 0 {
		return ""
	}

	output := ""
	for _, label := range labels {
		output += label + "\n"
	}
	return output
}
