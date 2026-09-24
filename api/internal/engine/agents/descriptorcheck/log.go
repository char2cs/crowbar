package descriptorcheck

import (
	"context"
	"log/slog"
)

// LogAll validates every descriptor under homeDir and logs each finding, so a
// descriptor the daemon refuses to enable says why at boot instead of simply
// missing from the provider list.
func LogAll(ctx context.Context, homeDir string) {
	reports, err := ValidateAll(homeDir)
	if err != nil {
		slog.WarnContext(ctx, "agent: validate descriptors", "err", err)
		return
	}
	for _, r := range reports {
		for _, f := range r.Findings {
			level := slog.LevelWarn
			msg := "agent: descriptor warning"
			if f.Severity == SeverityError {
				level, msg = slog.LevelError, "agent: descriptor blocked"
			}
			slog.Log(ctx, level, msg, "provider", r.ID, "source", r.Source, "rule", f.Rule,
				"path", f.Path, "line", f.Line, "finding", f.Message, "hint", f.Hint)
		}
	}
}
