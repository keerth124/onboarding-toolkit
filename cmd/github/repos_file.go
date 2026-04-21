package github

import (
	"fmt"
	"os"
	"strings"
)

func loadRepoNames(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading repos file %s: %w", path, err)
	}

	var repos []string
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			return nil, fmt.Errorf("%s:%d contains whitespace; use one repo name per line", path, lineNo+1)
		}
		repos = append(repos, line)
	}
	return repos, nil
}
