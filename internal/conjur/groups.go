package conjur

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GroupMemberBody is the JSON body for POST /api/groups/{id}/members.
type GroupMemberBody struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// writeGroupMembersArtifact writes 03-add-group-members.jsonl.
// Each line is a JSON object suitable as the body for a single membership addition.
func writeGroupMembersArtifact(authnName string, hosts []WorkloadHost, cfg GenerateConfig) (string, error) {
	groupID := appsGroupID(authnName)

	destDir := filepath.Join(cfg.WorkDir, "api")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating api dir: %w", err)
	}

	path := filepath.Join(destDir, "03-add-group-members.jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating group members file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	for _, h := range hosts {
		entry := GroupMemberBody{
			ID:   h.FullPath,
			Kind: "workload",
		}
		if err := enc.Encode(entry); err != nil {
			return "", fmt.Errorf("encoding group member: %w", err)
		}
	}

	return groupID, nil
}
