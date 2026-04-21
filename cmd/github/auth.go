package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func resolveGitHubToken(ctx context.Context, explicitToken string) (string, error) {
	if explicitToken != "" {
		return explicitToken, nil
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("GitHub token required: install GitHub CLI and run 'gh auth login', pass --token, or set GITHUB_TOKEN")
	}

	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("GitHub CLI is installed but not authenticated; run 'gh auth login' and 'gh auth refresh -s repo,read:org', pass --token, or set GITHUB_TOKEN")
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("GitHub CLI returned an empty token; run 'gh auth refresh -s repo,read:org', pass --token, or set GITHUB_TOKEN")
	}
	return token, nil
}
