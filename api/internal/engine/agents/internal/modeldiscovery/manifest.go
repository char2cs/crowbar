package modeldiscovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

type manifestDoc struct {
	updatedAt time.Time
	root      any
}

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
	if cachePath != "" && (!diskOK || !doc.updatedAt.Before(diskDoc.updatedAt)) {
		if err := writeManifestCache(cachePath, raw); err != nil {
			slog.Warn("modeldiscovery: manifest cache not saved", "path", cachePath, "err", err)
		}
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
	updatedAt, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		updatedAt = time.Time{}
	}
	return manifestDoc{updatedAt: updatedAt, root: root}, true
}

func fetchManifest(ctx context.Context, url string, timeoutMS int) (_ []byte, err error) {
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
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agents: model manifest fetch: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
}

func manifestCachePath(homeDir, url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(homeDir, manifestCacheSubdir, hex.EncodeToString(sum[:8])+".json")
}

func writeManifestCache(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
