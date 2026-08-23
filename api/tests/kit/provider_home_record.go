//go:build integration

package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ProviderHomeRecord struct {
	codex  map[string]struct{}
	claude map[string]struct{}
}

func SnapshotProviderHomes() ProviderHomeRecord {
	home := realUserHome()
	return ProviderHomeRecord{
		codex:  codexProjectKeys(filepath.Join(home, ".codex", "config.toml")),
		claude: claudeProjectKeys(filepath.Join(home, ".claude.json")),
	}
}

func (p ProviderHomeRecord) Added(
	before ProviderHomeRecord,
) string {
	codex := addedKeys(before.codex, p.codex)
	claude := addedKeys(before.claude, p.claude)
	if len(codex) == 0 && len(claude) == 0 {
		return ""
	}
	var report strings.Builder
	report.WriteString("this test run wrote per-place records into the user's REAL provider homes.\n" +
		"Every test builds a throwaway repo, so each of these is permanent litter in a file " +
		"nothing prunes.\nThe harness isolates both homes (kit.IsolateProviderHomes); a leak " +
		"here means a CLI was spawned without it, or a provider changed where it records trust.\n")
	appendLeaks(&report, "~/.codex/config.toml", codex)
	appendLeaks(&report, "~/.claude.json", claude)
	return report.String()
}

func appendLeaks(
	report *strings.Builder,
	where string,
	keys []string,
) {
	if len(keys) == 0 {
		return
	}
	fmt.Fprintf(report, "  %s gained %d entry/entries:\n", where, len(keys))
	for _, key := range keys {
		fmt.Fprintf(report, "    %s\n", key)
	}
}

func addedKeys(
	before map[string]struct{},
	after map[string]struct{},
) []string {
	var added []string
	for key := range after {
		if _, ok := before[key]; !ok {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	return added
}

func codexProjectKeys(
	path string,
) map[string]struct{} {
	keys := map[string]struct{}{}

	blob, err := os.ReadFile(path)
	if err != nil {
		return keys
	}
	for _, line := range strings.Split(string(blob), "\n") {
		header := strings.TrimSpace(line)
		if strings.HasPrefix(header, "[projects.") && strings.HasSuffix(header, "]") {
			keys[header] = struct{}{}
		}
	}
	return keys
}

func claudeProjectKeys(
	path string,
) map[string]struct{} {
	keys := map[string]struct{}{}

	blob, err := os.ReadFile(path)
	if err != nil {
		return keys
	}
	var config struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(blob, &config); err != nil {
		return keys
	}
	for key := range config.Projects {
		keys[key] = struct{}{}
	}
	return keys
}
