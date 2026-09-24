package modeldiscovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/pathselect"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const (
	maxManifestBytes    = 4 << 20
	manifestCacheSubdir = "model-manifest-cache"
)

// manifestDoc is one candidate copy of a manifest document — decoded just
// enough to compare freshness before the real items_path walk.
type manifestDoc struct {
	updatedAt time.Time
	root      any
}

// ProbeManifest resolves a model.manifest: catalogue as the freshest of the
// embedded bundle, the last good on-disk cache, and (fetchEnabled) a live
// HTTP GET — compared by each candidate's own "updatedAt", never positional
// preference alone, so a fetch that regresses in time can never override a
// newer cache. A successful, non-regressing fetch is persisted to disk so a
// later offline run still has it. embedded is required (the one candidate
// that must always exist); homeDir=="" skips the disk-cache half entirely;
// fetchEnabled==false skips the network half entirely — either way this
// degrades to whatever candidates remain, never an error and never an
// emptied catalogue.
func ProbeManifest(
	ctx context.Context,
	man *spec.ModelManifestSpec,
	embedded []byte,
	homeDir string,
	fetchEnabled bool,
) ([]Model, error) {
	if man == nil {
		return nil, ErrUnsupported
	}
	best, ok := parseManifestDoc(embedded)
	if !ok {
		return nil, ErrMalformedOutput
	}

	cachePath, diskDoc, diskOK := readManifestCache(homeDir, man.URL)
	if diskOK && diskDoc.updatedAt.After(best.updatedAt) {
		best = diskDoc
	}

	if fetchEnabled {
		best = mergeFetchedManifest(ctx, man, cachePath, best, diskDoc, diskOK)
	}

	return mapManifestDoc(best, man), nil
}

// readManifestCache returns the cache path for url — empty for a homeDir-less
// caller, which skips the disk half entirely — and the last good document
// parsed from it, if the file exists and still parses.
func readManifestCache(homeDir, url string) (string, manifestDoc, bool) {
	if homeDir == "" {
		return "", manifestDoc{}, false
	}
	cachePath := manifestCachePath(homeDir, url)
	//nolint:gosec // G304: filename is a sha256 digest under homeDir, never caller text.
	cached, err := os.ReadFile(cachePath)
	if err != nil {
		return cachePath, manifestDoc{}, false
	}
	doc, ok := parseManifestDoc(cached)
	if !ok {
		return cachePath, manifestDoc{}, false
	}
	return cachePath, doc, true
}

// mergeFetchedManifest GETs man.URL and returns the fresher of best and the
// fetched document, persisting the fetch to cachePath on the way. A failed or
// unparseable fetch leaves best exactly as it was.
func mergeFetchedManifest(
	ctx context.Context,
	man *spec.ModelManifestSpec,
	cachePath string,
	best, diskDoc manifestDoc,
	diskOK bool,
) manifestDoc {
	raw, err := fetchManifest(ctx, man.URL, man.EffectiveTimeoutMS())
	if err != nil {
		return best
	}
	doc, ok := parseManifestDoc(raw)
	if !ok {
		return best
	}
	if doc.updatedAt.After(best.updatedAt) {
		best = doc
	}
	// Never regress the disk cache: only overwrite it when the fetch is at
	// least as fresh as what is already there.
	if cachePath != "" && (!diskOK || !doc.updatedAt.Before(diskDoc.updatedAt)) {
		_ = writeManifestCache(cachePath, raw)
	}
	return best
}

func mapManifestDoc(doc manifestDoc, man *spec.ModelManifestSpec) []Model {
	rows := pathselect.Select([]any{doc.root}, man.ItemsPath)
	return mapModels(rows, itemMapping{KeepWhen: man.KeepWhen, Item: man.Item})
}

func parseManifestDoc(raw []byte) (manifestDoc, bool) {
	if len(raw) == 0 {
		return manifestDoc{}, false
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return manifestDoc{}, false
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return manifestDoc{}, false
	}
	ts, _ := scalarString(obj["updatedAt"])
	updatedAt, _ := time.Parse(time.RFC3339, ts)
	return manifestDoc{updatedAt: updatedAt, root: root}, true
}

func fetchManifest(ctx context.Context, url string, timeoutMS int) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("agents: model manifest url is empty")
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agents: model manifest fetch: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
}

// manifestCachePath is keyed by the URL's own hash rather than the
// provider id: the descriptor, not Go, owns which provider(s) use this
// source, and a future second manifest URL must not collide with this one.
func manifestCachePath(homeDir, url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(homeDir, manifestCacheSubdir, hex.EncodeToString(sum[:8])+".json")
}

func writeManifestCache(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644) //nolint:gosec // cache file, not a secret
}
