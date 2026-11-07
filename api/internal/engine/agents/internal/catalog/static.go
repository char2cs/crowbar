package catalog

import (
	"bufio"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// maxCommandFileBytes bounds how much of a command file is read for its description.
const maxCommandFileBytes = 64 << 10

// staticItems are the descriptor's built-ins and the workspace's own command
// files: known without running the CLI, so a failed probe never hides them.
func staticItems(sc *spec.SlashCatalogSpec, cwd string) []models.SlashCatalogItem {
	items := make([]models.SlashCatalogItem, 0, len(sc.Builtins))
	for _, b := range sc.Builtins {
		items = append(items, staticItem(models.CatalogItemKindBuiltin, b.Name, b.Description, "", b.Terminal))
	}
	// An os.Root resolves every path inside the workspace: a symlink out of it fails to open.
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return items
	}
	defer closeRead(root)
	for _, dir := range sc.Workspace {
		items = append(items, workspaceItems(root, dir)...)
	}
	return items
}

func workspaceItems(root *os.Root, dir spec.CatalogWorkspaceDir) []models.SlashCatalogItem {
	names := dirNames(root, dir.Dir)
	items := make([]models.SlashCatalogItem, 0, len(names))
	for _, name := range names {
		kind, file, stem := models.CatalogItemKindCommand, path.Join(dir.Dir, name), strings.TrimSuffix(name, ".md")
		if dir.Entry != "" {
			kind, file, stem = models.CatalogItemKindSkill, path.Join(dir.Dir, name, dir.Entry), name
		} else if !strings.HasSuffix(name, ".md") {
			continue
		}
		meta, ok := readMeta(root, file)
		if !ok {
			continue
		}
		items = append(items, staticItem(kind, cmp.Or(meta.name, stem), meta.description, dir.Source, false))
	}
	return items
}

func dirNames(root *os.Root, dir string) []string {
	f, err := root.Open(dir)
	if err != nil {
		return nil
	}
	defer closeRead(f)
	names, err := f.Readdirnames(spec.MaxCatalogItems)
	if err != nil && len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}

type commandMeta struct{ name, description string }

// readMeta reads a command's front matter `name:` and `description:`; without a
// description, the first line of the body stands for one, as the CLI shows it.
func readMeta(root *os.Root, file string) (commandMeta, bool) {
	f, err := root.Open(file)
	if err != nil {
		return commandMeta{}, false
	}
	defer closeRead(f)
	if info, statErr := f.Stat(); statErr != nil || !info.Mode().IsRegular() {
		return commandMeta{}, false
	}
	var meta commandMeta
	scanner := bufio.NewScanner(io.LimitReader(f, maxCommandFileBytes))
	inFront := false
	for line := 0; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		switch {
		case line == 0 && text == "---":
			inFront = true
		case inFront && text == "---":
			inFront = false
		case inFront:
			meta.frontField(text)
		case meta.description == "" && text != "":
			meta.description = strings.TrimSpace(strings.TrimLeft(text, "#"))
			return meta, true
		}
	}
	return meta, true
}

func (m *commandMeta) frontField(line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	switch strings.TrimSpace(key) {
	case "name":
		m.name = value
	case "description":
		m.description = value
	}
}

func staticItem(kind, name, description, source string, terminal bool) models.SlashCatalogItem {
	name = normalize.TruncateRunes(normalize.StripComposerControls(strings.TrimSpace(name)), normalize.MaxLabelRunes)
	insert := "/" + name + " "
	hash := sha256.Sum256([]byte(kind + "\x00" + source + "\x00" + insert))
	return models.SlashCatalogItem{
		ID:    kind + "-" + hex.EncodeToString(hash[:8]),
		Kind:  kind,
		Label: "/" + name,
		Description: normalize.Redact(normalize.TruncateBytes(
			normalize.StripControls(description), normalize.MaxDescriptionByte,
		)),
		InsertText: insert,
		Source:     normalize.Source(source),
		Terminal:   terminal,
	}
}

// closeRead closes what was opened only for reading: a failure loses nothing.
func closeRead(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Debug("agents: catalog: close a workspace command file", "err", err)
	}
}
