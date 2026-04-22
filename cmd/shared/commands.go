package shared

import (
	"fmt"
	"os"

	"github.com/cyberark/conjur-onboard/internal/core"
	"github.com/spf13/cobra"
)

type ApplyCommandOptions struct {
	PlatformID      string
	RollbackCommand string
	Long            string
}

type ValidateCommandOptions struct {
	PlatformID string
	Long       string
}

type RollbackCommandOptions struct {
	PlatformID string
	Long       string
}

func NewApplyCmd(flags GlobalFlags, opts ApplyCommandOptions) *cobra.Command {
	var conn ConjurConnectionFlags
	var skipValidate bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Execute the generated API plan against a Conjur endpoint",
		Long:  opts.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := conn.ValidateEndpointRequired(); err != nil {
				return err
			}
			if conn.Username == "" && !flags.IsDryRun() {
				return fmt.Errorf("--username is required")
			}

			apiKey := os.Getenv("CONJUR_API_KEY")
			if apiKey == "" && !flags.IsDryRun() {
				return fmt.Errorf("CONJUR_API_KEY environment variable is required")
			}

			wd, err := flags.EnsureWorkDir(opts.PlatformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			plan, err := core.LoadPlan(wd)
			if err != nil {
				return fmt.Errorf("loading plan.json: %w (run 'generate' first)", err)
			}

			var client core.APIClient
			if !flags.IsDryRun() {
				client, err = conn.NewClient(apiKey, flags.IsVerbose())
				if err != nil {
					return fmt.Errorf("conjur client: %w", err)
				}
			}

			result, err := core.Apply(cmd.Context(), core.ApplyConfig{
				WorkDir:      wd,
				Plan:         plan,
				Client:       client,
				DryRun:       flags.IsDryRun(),
				Verbose:      flags.IsVerbose(),
				SkipValidate: skipValidate,
			})
			if err != nil {
				rollback := opts.RollbackCommand
				if rollback == "" {
					rollback = "conjur-onboard rollback"
				}
				return fmt.Errorf("apply failed: %w\n\nTo roll back: %s", err, rollback)
			}

			fmt.Printf("\nApply complete\n")
			fmt.Printf("  Authenticator created : %s\n", result.AuthenticatorName)
			fmt.Printf("  Workloads created     : %d\n", result.WorkloadsCreated)
			fmt.Printf("  Group memberships     : %d\n", result.MembershipsAdded)
			fmt.Printf("  Log written to        : %s/apply-log.json\n", wd)
			return nil
		},
	}

	AddConjurConnectionFlags(cmd, &conn)
	cmd.Flags().BoolVar(&skipValidate, "skip-validate", false, "Skip pre-flight validation (not recommended)")

	return cmd
}

func NewValidateCmd(flags GlobalFlags, opts ValidateCommandOptions) *cobra.Command {
	var conn ConjurConnectionFlags

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run non-mutating checks against a generated API plan",
		Long:  opts.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := conn.ValidateEndpointRequired(); err != nil {
				return err
			}
			if conn.Username == "" && !flags.IsDryRun() {
				return fmt.Errorf("--username is required")
			}

			apiKey := os.Getenv("CONJUR_API_KEY")
			if apiKey == "" && !flags.IsDryRun() {
				return fmt.Errorf("CONJUR_API_KEY environment variable is required")
			}

			wd, err := flags.EnsureWorkDir(opts.PlatformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			plan, err := core.LoadPlan(wd)
			if err != nil {
				return fmt.Errorf("loading plan.json: %w (run 'generate' first)", err)
			}

			var client core.APIClient
			if !flags.IsDryRun() {
				client, err = conn.NewClient(apiKey, flags.IsVerbose())
				if err != nil {
					return fmt.Errorf("conjur client: %w", err)
				}
			}

			result, err := core.Validate(cmd.Context(), core.ValidateConfig{
				WorkDir: wd,
				Plan:    plan,
				Client:  client,
				DryRun:  flags.IsDryRun(),
				Verbose: flags.IsVerbose(),
			})
			if err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}

			fmt.Printf("Validation complete\n")
			fmt.Printf("  Operations checked : %d\n", result.Checked)
			fmt.Printf("  Log written to     : %s/validate-log.json\n", wd)
			for _, warning := range result.Warnings {
				fmt.Printf("  Warning            : %s\n", warning)
			}
			return nil
		},
	}

	AddConjurConnectionFlags(cmd, &conn)

	return cmd
}

func NewRollbackCmd(flags GlobalFlags, opts RollbackCommandOptions) *cobra.Command {
	var conn ConjurConnectionFlags
	var confirm bool

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Reverse successful operations from apply-log.json",
		Long:  opts.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := conn.ValidateEndpointRequired(); err != nil {
				return err
			}
			if conn.Username == "" && !flags.IsDryRun() {
				return fmt.Errorf("--username is required")
			}

			apiKey := os.Getenv("CONJUR_API_KEY")
			if apiKey == "" && !flags.IsDryRun() {
				return fmt.Errorf("CONJUR_API_KEY environment variable is required")
			}

			wd, err := flags.EnsureWorkDir(opts.PlatformID)
			if err != nil {
				return fmt.Errorf("work dir: %w", err)
			}

			plan, err := core.LoadPlan(wd)
			if err != nil {
				return fmt.Errorf("loading plan.json: %w", err)
			}

			var client core.APIClient
			if !flags.IsDryRun() {
				client, err = conn.NewClient(apiKey, flags.IsVerbose())
				if err != nil {
					return fmt.Errorf("conjur client: %w", err)
				}
			}

			result, err := core.Rollback(cmd.Context(), core.RollbackConfig{
				WorkDir: wd,
				Plan:    plan,
				Client:  client,
				DryRun:  flags.IsDryRun(),
				Confirm: confirm,
				Verbose: flags.IsVerbose(),
			})
			if err != nil {
				return fmt.Errorf("rollback failed: %w", err)
			}

			fmt.Printf("\nRollback complete\n")
			fmt.Printf("  Operations run : %d\n", result.OperationsRun)
			fmt.Printf("  Skipped        : %d\n", result.Skipped)
			fmt.Printf("  Log written to : %s/rollback-log.json\n", wd)
			if !flags.IsDryRun() {
				fmt.Printf("  Apply log moved: %s/apply-log.rolled-back.json\n", wd)
			}
			return nil
		},
	}

	AddConjurConnectionFlags(cmd, &conn)
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive rollback operations")

	return cmd
}
