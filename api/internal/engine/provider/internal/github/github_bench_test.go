package github

import (
	"encoding/json"
	"fmt"
	"testing"
)

// BenchmarkParsePRList benchmarks JSON parsing of a realistic 50-PR gh pr list output.
func BenchmarkParsePRList(b *testing.B) {
	payload := generatePRListPayload(50)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		prs, err := parsePRList(payload)
		if err != nil {
			b.Fatal(err)
		}
		if len(prs) != 50 {
			b.Fatalf("expected 50 PRs, got %d", len(prs))
		}
	}
}

// BenchmarkSelectBestPR benchmarks PR selection from a 50-PR list.
func BenchmarkSelectBestPR(b *testing.B) {
	payload := generatePRListPayload(50)
	prs, err := parsePRList(payload)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		best := selectBestPR(prs)
		if best == nil {
			b.Fatal("expected a best PR")
		}
	}
}

// generatePRListPayload creates a realistic JSON payload of n PRs.
func generatePRListPayload(
	n int,
) []byte {
	type prEntry struct {
		Number      int    `json:"number"`
		State       string `json:"state"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		BaseRefName string `json:"baseRefName"`
	}

	states := []string{"OPEN", "MERGED", "CLOSED"}
	bases := []string{"main", "develop", "release/v2"}

	prs := make([]prEntry, n)
	for i := range n {
		state := states[i%len(states)]
		base := bases[i%len(bases)]
		prs[i] = prEntry{
			Number:      i + 1,
			State:       state,
			URL:         fmt.Sprintf("https://github.com/owner/repo/pull/%d", i+1),
			Title:       fmt.Sprintf("Pull request number %d: implement feature %d with extensive description", i+1, i+1),
			BaseRefName: base,
		}
	}

	data, _ := json.Marshal(prs)
	return data
}
