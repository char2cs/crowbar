// Command protogen generates the Crowbar wire DTOs — Rust serde structs and
// TypeScript declarations — from the Go daemon's gin handlers, plus a JSON
// manifest of every endpoint it resolved and every one it could not.
//
// Usage:
//
//	protogen -daemon-root ./api -out-rust ./native/crates/crowbar-proto/src/generated \
//	         -out-ts ./web/src/lib/api/generated -manifest ./native/protogen.manifest.json
//
// The generated files are the single source of truth for both apps: a field
// that drifts in the Go handler becomes a compile error in Rust and in
// TypeScript instead of a silent undefined at runtime.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/char2cs/crowbar/native/tools/protogen/internal/gen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "protogen: %v\n", err)
		os.Exit(1)
	}
}

// run parses the flags, generates, and writes.
func run() error {
	var (
		daemonRoot = flag.String("daemon-root", "api",
			"path to the Go daemon module root (the api/ directory)")
		outRust = flag.String("out-rust", "",
			"directory to write the generated Rust modules into")
		outTS = flag.String("out-ts", "",
			"directory to write the generated TypeScript modules into")
		manifestPath = flag.String("manifest", "",
			"path to write the JSON manifest to")
		quiet = flag.Bool("quiet", false, "suppress the coverage summary on stderr")
	)
	flag.Parse()

	root, err := filepath.Abs(*daemonRoot)
	if err != nil {
		return fmt.Errorf("resolve -daemon-root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("-daemon-root %s is not a Go module root: %w", root, err)
	}

	result, err := gen.Generate(gen.DefaultOptions(root))
	if err != nil {
		return err
	}

	if *outRust != "" {
		if err := writeTree(*outRust, gen.EmitRust(result), ".rs"); err != nil {
			return err
		}
	}
	if *outTS != "" {
		if err := writeTree(*outTS, gen.EmitTS(result), ".ts"); err != nil {
			return err
		}
	}
	if *manifestPath != "" {
		data, err := gen.MarshalManifest(gen.BuildManifest(result))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*manifestPath), 0o755); err != nil {
			return fmt.Errorf("create manifest dir: %w", err)
		}
		if err := os.WriteFile(*manifestPath, data, 0o644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	}
	if !*quiet {
		fmt.Fprint(os.Stderr, summary(result))
	}
	return nil
}

// writeTree writes the emitted files into dir, first deleting the files a
// previous run left behind so a removed type does not linger.
func writeTree(
	dir string,
	files map[string]string,
	ext string,
) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := pruneGenerated(dir, ext); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// pruneGenerated removes files in dir that carry protogen's generated header.
// Anything hand-written in the same directory is left alone.
func pruneGenerated(
	dir string,
	ext string,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !strings.HasPrefix(string(data), gen.GeneratedHeader) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

// summary renders the coverage numbers a run measured.
func summary(
	r *gen.Result,
) string {
	s := gen.ComputeStats(r)
	var b strings.Builder
	fmt.Fprintf(&b, "protogen coverage\n")
	fmt.Fprintf(&b, "  routes discovered      %d (%d distinct paths)\n", s.Endpoints, s.DistinctPaths)
	fmt.Fprintf(&b, "  handlers resolved      %d\n", s.HandlersResolved)
	fmt.Fprintf(&b, "  fully resolved         %d\n", s.FullyResolved)
	fmt.Fprintf(&b, "  request bodies         %d resolved / %d bound\n", s.RequestResolved, s.WithRequestBody)
	fmt.Fprintf(&b, "  response payloads      %d resolved / %d written\n", s.ResponseResolved, s.WithResponsePayload)
	fmt.Fprintf(&b, "  types emitted          %d in %d modules (%d incomplete)\n",
		s.Types, s.Modules, s.TypesIncomplete)
	kinds := make([]string, 0, len(s.TypesByKind))
	for k := range s.TypesByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "    %-20s %d\n", k, s.TypesByKind[k])
	}
	respKinds := make([]string, 0, len(s.ByResponseKind))
	for k := range s.ByResponseKind {
		respKinds = append(respKinds, k)
	}
	sort.Strings(respKinds)
	fmt.Fprintf(&b, "  response kinds\n")
	for _, k := range respKinds {
		fmt.Fprintf(&b, "    %-20s %d\n", k, s.ByResponseKind[k])
	}
	fmt.Fprintf(&b, "  diagnostics            %d (%d errors, %d warnings)\n",
		s.Unresolved, s.UnresolvedErrors, s.UnresolvedWarnings)
	counts := map[string]int{}
	for _, u := range r.Unresolved {
		counts[u.Category]++
	}
	cats := make([]string, 0, len(counts))
	for c := range counts {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool {
		if counts[cats[i]] != counts[cats[j]] {
			return counts[cats[i]] > counts[cats[j]]
		}
		return cats[i] < cats[j]
	})
	for _, c := range cats {
		fmt.Fprintf(&b, "    %-20s %d\n", c, counts[c])
	}
	return b.String()
}
