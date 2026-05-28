package main

import (
	"fmt"
	"strings"
)

func scopeExpression(scopes []string) (string, error) {
	if len(scopes) == 0 {
		return "", fmt.Errorf("at least one --scope is required")
	}

	positives := make([]string, 0, len(scopes))
	negatives := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return "", fmt.Errorf("scope must not be empty")
		}
		if strings.HasPrefix(scope, "-") {
			negatives = append(negatives, strings.TrimPrefix(scope, "-"))
			continue
		}
		positives = append(positives, scope)
	}
	if len(positives) == 0 {
		return "", fmt.Errorf("at least one positive --scope is required")
	}

	expr := unionExpression(positives)
	if len(negatives) > 0 {
		expr += " except " + unionExpression(negatives)
	}
	return expr, nil
}

func unionExpression(scopes []string) string {
	expr := scopes[0]
	for _, scope := range scopes[1:] {
		expr += " union " + scope
	}
	return expr
}
