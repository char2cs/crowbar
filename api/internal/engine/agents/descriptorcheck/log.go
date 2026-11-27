package descriptorcheck

import (
	"context"
	"log/slog"
)

func LogAll(ctx context.Context, homeDir string) {
	reports, err := ValidateAll(homeDir)
	if err != nil {
		slog.WarnContext(ctx, "agent: validate descriptors", "err", err)
		return
	}
	for _, r := range reports {
		if r.FellBack {
			slog.WarnContext(ctx, "agent: descriptor override refused; running the shipped descriptor",
				"provider", r.ID, "source", r.Source)
		}
		for _, f := range r.Findings {
			level := slog.LevelWarn
			msg := "agent: descriptor warning"
			switch {
			case r.FellBack && f.Severity == SeverityError:
				msg = "agent: descriptor override refused"
			case f.Severity == SeverityError:
				level, msg = slog.LevelError, "agent: descriptor blocked"
			}
			slog.Log(ctx, level, msg, "provider", r.ID, "source", r.Source, "rule", f.Rule,
				"path", f.Path, "line", f.Line, "finding", f.Message, "hint", f.Hint)
		}
	}
}
