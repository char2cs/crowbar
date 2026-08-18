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

// ProviderHomeRecord is the set of PER-PLACE records the vendor CLIs are holding in
// the user's real provider homes at one instant: the [projects."…"] stanzas in
// ~/.codex/config.toml and the "projects" keys in ~/.claude.json.
//
// It exists to be compared with itself across a test run. Those two maps are the only
// thing a test can make a vendor CLI append to a real provider home, they are keyed by
// a path that will never exist again once the run's temp repos are reaped, and nothing
// ever prunes them — so a run that adds one has leaked, and Added is how that is
// caught rather than noticed months later at 61KB and 414 stanzas.
//
// It counts KEYS and not bytes deliberately. A provider rewrites its own home for its
// own reasons all the time — codex refreshes models_cache.json, claude bumps a startup
// counter — and a guard that tripped on those would be noise. Only a new place is a
// leak this suite is responsible for.
type ProviderHomeRecord struct {
	codex  map[string]struct{}
	claude map[string]struct{}
}

// SnapshotProviderHomes reads the user's real provider homes and records which places
// they currently trust. A home that cannot be read contributes nothing, so a machine
// with no codex or no claude installed snapshots clean and compares clean.
func SnapshotProviderHomes() ProviderHomeRecord {
	home := realUserHome()
	return ProviderHomeRecord{
		codex:  codexProjectKeys(filepath.Join(home, ".codex", "config.toml")),
		claude: claudeProjectKeys(filepath.Join(home, ".claude.json")),
	}
}

// Added reports the places that appeared in the real provider homes since before, as a
// human-readable report naming each leaked key, or "" when nothing was added.
//
// It is deliberately one-sided: keys DISAPPEARING is not this guard's business (the
// user is free to prune their own config mid-run), and reporting it would turn an
// unrelated user action into a failed test run.
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

// codexProjectKeys scrapes the [projects."<path>"] table headers out of codex's TOML
// by line rather than by parsing it. The file is the user's own and may hold anything
// a future codex writes; a scrape reads the one construct this guard cares about and
// cannot fail the run over a stanza it does not understand.
func codexProjectKeys(
	path string,
) map[string]struct{} {
	keys := map[string]struct{}{}
	// #nosec G304 -- path is always a provider home under the login account's own
	// home directory, composed here rather than taken from any caller.
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
	// #nosec G304 -- path is always a provider home under the login account's own
	// home directory, composed here rather than taken from any caller.
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
