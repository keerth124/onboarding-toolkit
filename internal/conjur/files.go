package conjur

import (
	"fmt"
	"os"
	"path/filepath"
)

func removeAPIArtifact(workDir string, name string, description string) error {
	path := filepath.Join(workDir, "api", name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", description, err)
	}
	return nil
}
