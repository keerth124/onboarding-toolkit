package conjur

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cyberark/conjur-onboard/internal/core"
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

	var sb strings.Builder
	for _, h := range hosts {
		entry := GroupMemberBody{
			ID:   h.FullPath,
			Kind: "workload",
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return "", fmt.Errorf("encoding group member: %w", err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}

	destDir := filepath.Join(cfg.WorkDir, "api")
	if err := core.WriteText(destDir, "03-add-group-members.jsonl", sb.String()); err != nil {
		return "", fmt.Errorf("writing group members artifact: %w", err)
	}
	return groupID, nil
}
