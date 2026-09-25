package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

var errDescriptorBlocked = errors.New("descriptor check failed")

func newDescriptorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "descriptor",
		Short:        "Validate and test provider descriptors",
		SilenceUsage: true,
	}
	cmd.AddCommand(newDescriptorValidateCmd(), newDescriptorTestCmd())
	return cmd
}

func newDescriptorValidateCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "validate [file.yaml ...]",
		Short: "Check descriptors against every static rule (all installed ones when no file is given)",
		RunE: func(c *cobra.Command, files []string) error {
			reports, err := validateTargets(files)
			if err != nil {
				return err
			}
			return printReports(c.OutOrStdout(), reports, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func newDescriptorTestCmd() *cobra.Command {
	var (
		asJSON, isLive bool
		opts           descriptorcheck.LiveOptions
	)
	cmd := &cobra.Command{
		Use:   "test <provider-id|file.yaml>",
		Short: "Run a descriptor against its real CLI (--live); without --live, validate only",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			raw, err := descriptorBytes(args[0])
			if err != nil {
				return err
			}
			if !isLive {
				return printReports(c.OutOrStdout(), []descriptorcheck.Report{descriptorcheck.Validate(raw)}, asJSON)
			}
			ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			rep := descriptorcheck.Conform(ctx, raw, opts)
			return printLive(c.OutOrStdout(), rep, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&isLive, "live", false, "spawn the real CLI: binary, version, flags, TUI boot, hooks, resume, app-server")
	cmd.Flags().BoolVar(&opts.Turn, "turn", false, "with --live, run one real model turn (one billed request)")
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "with --live, boot the CLI here instead of a fresh temporary directory")
	return cmd
}

// descriptorBytes reads a descriptor file, or the installed descriptor for a
// provider id (the override under crowbar home, else the shipped one).
func descriptorBytes(target string) ([]byte, error) {
	if strings.HasSuffix(target, ".yaml") || strings.ContainsRune(target, os.PathSeparator) {
		raw, err := os.ReadFile(target) //nolint:gosec // a path the user named on the command line
		if err != nil {
			return nil, fmt.Errorf("descriptor: %w", err)
		}
		return raw, nil
	}
	reports, err := descriptorcheck.Sources(metadata.GetHomePath())
	if err != nil {
		return nil, err
	}
	for _, src := range reports {
		if src.ID == target {
			return src.Raw, nil
		}
	}
	return nil, fmt.Errorf("descriptor: no provider %q", target)
}

func validateTargets(files []string) ([]descriptorcheck.Report, error) {
	if len(files) == 0 {
		return descriptorcheck.ValidateAll(metadata.GetHomePath())
	}
	out := make([]descriptorcheck.Report, 0, len(files))
	for _, f := range files {
		raw, err := descriptorBytes(f)
		if err != nil {
			return nil, err
		}
		rep := descriptorcheck.Validate(raw)
		rep.Source = f
		out = append(out, rep)
	}
	return out, nil
}

func printReports(w io.Writer, reports []descriptorcheck.Report, asJSON bool) error {
	ok := true
	for _, r := range reports {
		ok = ok && r.OK()
	}
	if err := emit(w, reports, asJSON, func() {
		for _, r := range reports {
			writeReport(w, r)
		}
	}); err != nil {
		return err
	}
	if !ok {
		return errDescriptorBlocked
	}
	return nil
}

// emit writes v as JSON, or runs text for the human-readable form.
func emit(w io.Writer, v any, asJSON bool, text func()) error {
	if !asJSON {
		text()
		return nil
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("descriptor: %w", err)
	}
	return nil
}

func writeReport(w io.Writer, r descriptorcheck.Report) {
	status := "ok"
	if !r.OK() {
		status = "BLOCKED"
	}
	where := "shipped"
	if r.Source != "" {
		where = r.Source
	}
	_, _ = fmt.Fprintf(w, "%s (%s): %s\n", r.ID, where, status)
	for _, f := range r.Findings {
		_, _ = fmt.Fprintf(w, "  %s %s at %s (line %d): %s\n", f.Severity, f.Rule, f.Path, f.Line, f.Message)
		if f.Hint != "" {
			_, _ = fmt.Fprintf(w, "      hint: %s\n", f.Hint)
		}
	}
}

func printLive(w io.Writer, rep descriptorcheck.LiveReport, asJSON bool) error {
	if err := emit(w, rep, asJSON, func() {
		writeReport(w, rep.Report)
		for _, s := range rep.Steps {
			_, _ = fmt.Fprintf(w, "  %-4s %-15s %6.1fs  %s\n", s.Status, s.Name, s.Elapsed.Seconds(), s.Detail)
		}
	}); err != nil {
		return err
	}
	if !rep.OK() {
		return errDescriptorBlocked
	}
	return nil
}
