// Package shared contains CLI wiring shared by platform command packages.
package shared

import (
	"fmt"
	"time"

	"github.com/cyberark/conjur-onboard/internal/core"
)

// GlobalFlags are root-level flags shared by every platform command.
type GlobalFlags struct {
	WorkDir        *string
	NonInteractive *bool
	DryRun         *bool
	Verbose        *bool
}

// WorkDirFor returns the explicit work directory or a platform-specific default.
func (f GlobalFlags) WorkDirFor(platformID string) string {
	if f.WorkDir != nil && *f.WorkDir != "" {
		return *f.WorkDir
	}
	if platformID == "" {
		platformID = "platform"
	}
	return fmt.Sprintf("conjur-onboard-%s-%s", platformID, time.Now().Format("20060102-150405"))
}

// EnsureWorkDir creates and returns the work directory for a platform command.
func (f GlobalFlags) EnsureWorkDir(platformID string) (string, error) {
	return core.EnsureWorkDir(f.WorkDirFor(platformID))
}

func (f GlobalFlags) IsDryRun() bool {
	return f.DryRun != nil && *f.DryRun
}

func (f GlobalFlags) IsVerbose() bool {
	return f.Verbose != nil && *f.Verbose
}
