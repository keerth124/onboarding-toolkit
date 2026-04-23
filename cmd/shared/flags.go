// Package shared contains CLI wiring shared by platform command packages.
package shared

import (
	"fmt"
	"time"

	"github.com/cyberark/conjur-onboard/internal/appconfig"
	"github.com/cyberark/conjur-onboard/internal/core"
)

// GlobalFlags are root-level flags shared by every platform command.
type GlobalFlags struct {
	WorkDir        *string
	ConfigPath     *string
	ConfigExplicit *bool
	NonInteractive *bool
	DryRun         *bool
	Verbose        *bool
}

// WorkDirFor returns the explicit work directory or a platform-specific default.
func (f GlobalFlags) WorkDirFor(platformID string) string {
	if f.WorkDir != nil && *f.WorkDir != "" {
		return *f.WorkDir
	}
	if cfg, found, err := f.LoadConfig(); err == nil && found && cfg.WorkDir != "" {
		return cfg.WorkDir
	}
	if platformID == "" {
		platformID = "platform"
	}
	return fmt.Sprintf("conjur-onboard-%s-%s", platformID, time.Now().Format("20060102-150405"))
}

// EnsureWorkDir creates and returns the work directory for a platform command.
func (f GlobalFlags) EnsureWorkDir(platformID string) (string, error) {
	if _, _, err := f.LoadConfig(); err != nil {
		return "", err
	}
	return core.EnsureWorkDir(f.WorkDirFor(platformID))
}

func (f GlobalFlags) ConfigPathValue() string {
	if f.ConfigPath != nil && *f.ConfigPath != "" {
		return *f.ConfigPath
	}
	return appconfig.DefaultPath
}

func (f GlobalFlags) IsConfigExplicit() bool {
	if f.ConfigExplicit != nil && *f.ConfigExplicit {
		return true
	}
	return f.ConfigPath != nil && *f.ConfigPath != "" && *f.ConfigPath != appconfig.DefaultPath
}

func (f GlobalFlags) LoadConfig() (appconfig.Config, bool, error) {
	return appconfig.Load(f.ConfigPathValue(), f.IsConfigExplicit())
}

func (f GlobalFlags) IsDryRun() bool {
	return f.DryRun != nil && *f.DryRun
}

func (f GlobalFlags) IsVerbose() bool {
	return f.Verbose != nil && *f.Verbose
}

func (f GlobalFlags) IsNonInteractive() bool {
	return f.NonInteractive != nil && *f.NonInteractive
}
