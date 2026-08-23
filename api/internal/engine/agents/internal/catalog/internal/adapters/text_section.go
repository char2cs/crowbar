package adapters

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type textSection struct{}

func (textSection) Probe(
	ctx context.Context,
	s *spec.SlashCatalogSpec,
	runner models.ProbeRunner,
) (Result, error) {
	p := &s.Pipeline
	raw, err := runner.Run(ctx, p.Command)
	if err != nil {
		return Result{}, err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return Result{}, ErrMalformedOutput
	}

	pattern, _ := regexp.Compile(p.ItemPattern)

	limit := s.EffectiveMaxItems() + 1
	candidates := make([]Candidate, 0)
	sectionFound := false
	for _, value := range selectPath([]any{document}, p.TextPath) {
		text, ok := value.(string)
		if !ok {
			continue
		}
		for _, section := range literalSections(text, p.StartMarker, p.EndMarker) {
			sectionFound = true
			remaining := limit - len(candidates)
			if remaining <= 0 {
				break
			}
			candidates = append(candidates, matchSection(pattern, section, remaining)...)
		}
	}

	if !sectionFound {
		return Result{}, ErrMalformedOutput
	}
	return Result{Candidates: candidates}, nil
}

func matchSection(pattern *regexp.Regexp, section string, limit int) []Candidate {
	out := make([]Candidate, 0, limit)
	for _, match := range pattern.FindAllStringSubmatch(section, limit) {
		captures := namedCaptures(pattern, match)
		name := strings.TrimSpace(captures["name"])
		if name == "" {
			continue
		}
		out = append(out, Candidate{
			Name:        name,
			Description: strings.TrimSpace(captures["description"]),
			Source:      strings.TrimSpace(captures["source"]),
		})
	}
	return out
}
