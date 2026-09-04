# Chat Attachments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add positional chat attachments (pasted text blocks, images, files/CSV-as-table, Excalidraw diagrams) to the Plate-based chat composer and transcript, with no backend message-format change.

**Architecture:** Four attachment kinds encode into the existing flat markdown message text — text/Excalidraw inline via id-suffixed fenced code blocks, images/files via ordinary `![]()`/`[]()` pointing at a durable per-chat store. A new Go backend surface (upload + asset-serving endpoints, `chats/{chatId}/attachments/`) holds the durable bytes; at turn-dispatch time the runner materializes referenced files into a transient scratch directory inside the worktree (cleaned up on turn exit or boot reconciliation) so the provider CLI can read them without any cross-provider permission-grant machinery. On the frontend, a new `MarkdownAssetContext.Provider` resolves those references to real bytes for rendering, reusing already-registered Plate plugins (`MarkdownImageKit`, `CodeBlockPlugin`, `TablePlugin`) wherever possible.

**Tech Stack:** Go (Gin), React/TypeScript, Plate (`platejs`, `@platejs/markdown`, `@platejs/code-block`, `@platejs/dnd`), `nanoid`, `papaparse`, `@excalidraw/excalidraw`, Vitest, `go test`.

**Spec:** `docs/superpowers/specs/2026-09-04-chat-attachments-design.md`

## Global Constraints

- No backend wire-format change: `LedgerTurn.Text` stays a flat markdown string; attachments are encoded entirely within it.
- Durable attachment reference shape (hard requirement, both sides parse against this exact pattern): `chats/{chatId}/attachments/{filename}`.
- Attachment id shape (hard requirement): `/^[A-Za-z0-9_-]{6,}$/` — enforced both in the frontend fence-tag parser and the backend's `ValidID`.
- Fence-tag encoding: ` ```text-attachment:{id} ` and ` ```excalidraw:{id} ` — a bare tag with no valid id suffix (and, for excalidraw, content that doesn't parse as a scene) must render as an ordinary code block, never be treated as an attachment.
- Durable store size cap: 25 MiB per attachment (`repoattachments.MaxBytes`).
- Storage location: `{workspaceRoot}/chats/{chatId}/attachments/` (durable) and `{worktree}/.crowbar-attachments/{runnerId}/` (transient dispatch scratch, never gitignored — disclosed as a brief `git status` blip during an active turn, not hidden).
- Test file location per this repo's `CLAUDE.md`: frontend tests live in `web/src/__tests__/` mirroring `web/src/`, never co-located; use `@/` imports in test files. Component files are kebab-case, exported component names PascalCase.
- Frontend test runner: `bun`/`bunx vitest` (this repo uses bun, not npm/yarn).
- Backend test runner: `go test`.

---

# Phase 1: Backend — Storage, Endpoints, Dispatch Delivery

### Task 1: Worktree path helpers for the durable store and scratch dir

**Files:**
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath.go` (append after line 405, end of file)
- Test: `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go` (append)

**Interfaces:**
- Consumes: nothing new — `filepath`, `strings`, `os`, `log/slog`, `context` (already imported)
- Produces:
  - `func AttachmentsDir(chatsDir, chatID string) string`
  - `const AttachmentScratchDirName = ".crowbar-attachments"`
  - `func AttachmentScratchDir(worktree, runnerID string) string`
  - `func UnderWorktree(path, worktree string) bool`
  - `func RemoveUnderWorktree(ctx context.Context, worktree, target string)`

- [ ] **Step 1: Write the failing test**

```go
func TestAttachmentsDir(t *testing.T) {
	dir := AttachmentsDir("/crow/projects/p1/slug/branch/chats", "chat-1")
	assert.Equal(t, "/crow/projects/p1/slug/branch/chats/chat-1/attachments", dir)
}

func TestAttachmentScratchDir(t *testing.T) {
	dir := AttachmentScratchDir("/work/tree", "runner-1")
	assert.Equal(t, "/work/tree/.crowbar-attachments/runner-1", dir)
}

func TestUnderWorktree(t *testing.T) {
	assert.True(t, UnderWorktree("/work/tree/.crowbar-attachments/r1", "/work/tree"))
	assert.False(t, UnderWorktree("/work/tree", "/work/tree"), "worktree itself is never under worktree")
	assert.False(t, UnderWorktree("/work/tree-other/x", "/work/tree"), "string-prefix sibling is not nested")
	assert.False(t, UnderWorktree("", "/work/tree"))
	assert.False(t, UnderWorktree("/work/tree/x", ""))
}

func TestRemoveUnderWorktree(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	target := filepath.Join(worktree, ".crowbar-attachments", "r1")
	require.NoError(t, os.MkdirAll(target, 0o755))

	RemoveUnderWorktree(context.Background(), worktree, target)

	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveUnderWorktree_RefusesAPathOutsideTheWorktree(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o755))

	RemoveUnderWorktree(context.Background(), worktree, outside)

	assert.DirExists(t, outside, "must never remove a path outside the worktree")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/internal/worktreepath/... -run 'TestAttachmentsDir|TestAttachmentScratchDir|TestUnderWorktree|TestRemoveUnderWorktree' -v`
Expected: FAIL — `undefined: AttachmentsDir` (and the other three symbols)

- [ ] **Step 3: Write minimal implementation**

```go
// AttachmentsDir returns a chat's durable attachment store: the directory the
// upload endpoint writes into and the asset-serving GET endpoint reads from.
//
// Path: <chatsDir>/<chatID>/attachments. It nests under the same chats/
// sibling directory ChatsDir already roots the ledger and RunnerDir under
// (never inside the git worktree, so an upload never appears in git status),
// keyed by the chat's own id — unlike RunnerDir, an attachment's chat pointer
// is never erased, and deleting the chat already removes chats/<chatID>
// wholesale, so there is no separate cleanup path to get right here.
func AttachmentsDir(chatsDir, chatID string) string {
	return filepath.Join(chatsDir, chatID, "attachments")
}

// AttachmentScratchDirName is the dot-prefixed directory, at a workspace's
// worktree root, that holds per-runner scratch copies of attachments
// materialized for CLI dispatch. Deliberately absent from the worktree's own
// .gitignore — writing to a file the user owns and tracks is not this
// daemon's business, least of all for a directory that only exists for the
// seconds-to-minutes a turn referencing an attachment is in flight (chat
// attachments design spec, "Storage & agent delivery").
const AttachmentScratchDirName = ".crowbar-attachments"

// AttachmentScratchDir returns runnerID's scratch attachment directory inside
// worktree, keyed by runnerID for the same reason RunnerDir is: it is what a
// runner's onExit callback already has in hand on a clean death, and what
// boot reconciliation can re-derive from a bare dead-runner row with no chat
// pointer needed.
func AttachmentScratchDir(worktree, runnerID string) string {
	return filepath.Join(worktree, AttachmentScratchDirName, runnerID)
}

// UnderWorktree reports whether path is strictly nested under worktree — the
// scratch-attachment analogue of UnderHome, for a path living INSIDE the git
// worktree rather than under crowbar home.
func UnderWorktree(path, worktree string) bool {
	if path == "" || worktree == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(worktree, "/")+"/")
}

// RemoveUnderWorktree deletes target only when it is strictly under worktree,
// and never fails the caller — the scratch-attachment analogue of
// RemoveUnderHome. RunnerDir's own reap helper checks crowbarHome; a scratch
// attachment copy lives INSIDE the git worktree instead, so it needs this
// separate boundary rather than reusing that one.
func RemoveUnderWorktree(
	ctx context.Context,
	worktree string,
	target string,
) {
	if !UnderWorktree(target, worktree) {
		slog.WarnContext(ctx, "agent: refusing to rm attachment scratch path outside the worktree (skipping)",
			"target", target, "worktree", worktree)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		slog.WarnContext(ctx, "agent: reap attachment scratch path", "target", target, "err", err)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/internal/worktreepath/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/internal/worktreepath/
git commit -m "feat(worktreepath): add attachment durable-store and scratch-dir helpers"
```

---

### Task 2: Attachment durable-store repository package

**Files:**
- Create: `api/internal/app/repositories/chat/attachments/attachments.go`
- Test: `api/internal/app/repositories/chat/attachments/attachments_test.go`

**Interfaces:**
- Consumes: `apperr.ErrInvalidArgument`, `apperr.ErrConflict`, `safepath.ErrFileTooLarge` (existing sentinels)
- Produces:
  - `const MaxBytes = 25 << 20`
  - `var ErrNotFound error`
  - `func ValidID(id string) bool`
  - `func Store(dir, id, originalName string, data []byte) (fileName, contentType string, err error)`
  - `func Read(dir, fileName string) (data []byte, contentType string, err error)`
  - `func SyntheticName(contentType string, now func() string) string`
  - `func ContentType(data []byte) string`

- [ ] **Step 1: Write the failing test**

```go
package attachments_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
}

func TestStore_WritesTheIDPrefixedFilename(t *testing.T) {
	dir := t.TempDir()
	fileName, contentType, err := attachments.Store(dir, "ab12", "photo.png", pngBytes())
	require.NoError(t, err)
	assert.Equal(t, "ab12-photo.png", fileName)
	assert.Equal(t, "image/png", contentType)
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
}

func TestStore_SanitizesAPathTraversalOriginalName(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "../../etc/passwd", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "ab12-passwd", fileName)
}

func TestStore_CollapsesSpacesAndParensInTheOriginalName(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "my photo (1).png", []byte("x"))
	require.NoError(t, err)
	assert.NotContains(t, fileName, " ")
	assert.NotContains(t, fileName, "(")
}

func TestStore_RefusesAnInvalidID(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "../escape", "a.png", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_RefusesABlankOriginalName(t *testing.T) {
	_, _, err := attachments.Store(t.TempDir(), "ab12", "", []byte("x"))
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestStore_RefusesOversizeData(t *testing.T) {
	oversized := make([]byte, attachments.MaxBytes+1)
	_, _, err := attachments.Store(t.TempDir(), "ab12", "big.bin", oversized)
	assert.ErrorIs(t, err, safepath.ErrFileTooLarge)
}

func TestStore_RefusesACollidingFilename(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Store(dir, "ab12", "photo.png", []byte("first"))
	require.NoError(t, err)
	_, _, err = attachments.Store(dir, "ab12", "photo.png", []byte("second"))
	assert.ErrorIs(t, err, apperr.ErrConflict)
}

func TestRead_RoundTripsStoredBytes(t *testing.T) {
	dir := t.TempDir()
	fileName, _, err := attachments.Store(dir, "ab12", "photo.png", pngBytes())
	require.NoError(t, err)

	data, contentType, err := attachments.Read(dir, fileName)
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
	assert.Equal(t, "image/png", contentType)
}

func TestRead_MissingFileIsNotFound(t *testing.T) {
	_, _, err := attachments.Read(t.TempDir(), "no-such-file.png")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestRead_RefusesAFileNameCarryingASeparator(t *testing.T) {
	dir := t.TempDir()
	_, _, err := attachments.Read(dir, "../outside.png")
	assert.ErrorIs(t, err, attachments.ErrNotFound)
}

func TestSyntheticName_UsesTheSniffedExtension(t *testing.T) {
	name := attachments.SyntheticName("image/png", func() string { return "20260904T120000" })
	assert.Equal(t, "pasted-image-20260904T120000.png", name)
}

func TestContentType_SVGGetsItsOwnMIMEType(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	assert.Equal(t, "image/svg+xml", attachments.ContentType(svg))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/repositories/chat/attachments/... -v`
Expected: FAIL — package does not exist / `undefined: attachments.Store`

- [ ] **Step 3: Write minimal implementation**

```go
// Package attachments is the durable per-chat attachment store: validating,
// naming, writing and serving the files a chat's composer sends alongside a
// message — images, PDFs, and anything too large or too binary to inline in
// the message text itself (chat attachments design spec).
//
// It is the storage half of a handler -> usecase -> storage layering that
// mirrors agentactivity's own Payload seam (ReadToolPayload -> EventStore.Payload
// -> content store): chat/handlers owns the HTTP ingestion shapes, the chat
// usecase resolves WHICH directory a chat's attachments live in, and this
// package owns what happens once both bytes and a destination are known.
package attachments

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// MaxBytes caps a single stored attachment at 25 MiB.
//
// Icons cap at 2 MiB (a small avatar) and the tool-call content store caps at
// 8 MiB (bounded provider output); a chat attachment is neither — it is a
// user-picked image, PDF or CSV export, and a modern phone photo alone can
// clear 10-15 MiB before a scanned PDF is even in the picture. 25 MiB matches
// this codebase's own existing ceiling for a single file read
// (safepath.ErrFileTooLarge, the workspace file-read cap) rather than
// inventing a new number — one large-file boundary to reason about, not two.
const MaxBytes = 25 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidID reports whether id is safe to use as the shortid half of a stored
// filename. The frontend mints these with nanoid; this is the daemon's own
// check, never a trust of the client's generator.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var unsafeFileNameChars = regexp.MustCompile(`[\s()\[\]]+`)

// sanitizeName reduces name to a safe, markdown-safe base filename: no path
// separators, no "." or "..", and no whitespace/parens/brackets — a stored
// filename becomes the trailing segment of a ![]()/[]() reference (design
// spec), so an unescaped paren or a space in the ORIGINAL name would corrupt
// the very markdown syntax storing it. Collapsed to "_", not rejected: "my
// photo (1).png" is an entirely ordinary name to refuse outright.
func sanitizeName(name string) string {
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	return unsafeFileNameChars.ReplaceAllString(base, "_")
}

// Store writes data as a new attachment under dir (a chat's AttachmentsDir),
// named "<id>-<sanitized originalName>", creating dir if needed.
//
// Refuses with apperr.ErrInvalidArgument for an invalid id or a blank
// original name (a caller with no filename — a clipboard paste — must
// synthesize one first, see SyntheticName), with safepath.ErrFileTooLarge
// over MaxBytes, and with apperr.ErrConflict when the target filename
// already exists: the id is client-generated per attachment, so a collision
// means the SAME id was submitted twice, and overwriting silently would let
// a second upload invisibly replace bytes a message may already reference.
func Store(
	dir, id, originalName string,
	data []byte,
) (fileName, contentType string, err error) {
	if !ValidID(id) {
		return "", "", fmt.Errorf("attachments: invalid id: %w", apperr.ErrInvalidArgument)
	}
	base := sanitizeName(originalName)
	if base == "" {
		return "", "", fmt.Errorf("attachments: original name required: %w", apperr.ErrInvalidArgument)
	}
	if len(data) > MaxBytes {
		return "", "", fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
	}
	fileName = id + "-" + base
	//nolint:gosec // G301: 0o700 matches RunnerDir's own perms for daemon-managed chat state.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("attachments: create directory: %w", err)
	}
	dest := filepath.Join(dir, fileName)
	if _, statErr := os.Stat(dest); statErr == nil {
		return "", "", fmt.Errorf("attachments: %q already exists: %w", fileName, apperr.ErrConflict)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", "", fmt.Errorf("attachments: stat destination: %w", statErr)
	}
	//nolint:gosec // G306: served back over HTTP to this same daemon's own clients; 0o600 keeps it off other local users.
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", "", fmt.Errorf("attachments: write: %w", err)
	}
	return fileName, ContentType(data), nil
}

// SyntheticName builds a fallback filename for an attachment with no original
// name — a clipboard image paste, which carries raw bytes and a sniffed
// content type but nothing a browser or OS ever called a "filename". now is
// injected so the name is deterministic under test. Wired into the upload
// handler (Task 4) as a defensive fallback for a blank filename — the client
// should always send a real name where it can, but this closes the gap when
// it can't.
func SyntheticName(contentType string, now func() string) string {
	return "pasted-image-" + now() + extensionFor(contentType)
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}

// ContentType sniffs a Content-Type from data, the same way
// icons.ContentType does (deliberately re-implemented here rather than
// imported: icons lives in the API layer, and this repository package must
// not depend upward on it). http.DetectContentType has no SVG signature —
// it sniffs SVG as text/* — so SVG is special-cased.
func ContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if strings.Contains(string(head), "<svg") {
		return "image/svg+xml"
	}
	return ct
}

// ErrNotFound reports that no attachment exists at the requested reference.
var ErrNotFound = errors.New("attachments: not found")

// Read returns fileName's bytes and sniffed content type from dir, or
// ErrNotFound. Stat-rejected and capped exactly like icons.Serve: dir holds
// files this daemon wrote, but a corrupted or replaced file must never cause
// an unbounded read. fileName must be a bare filename with no separators —
// the shape Store produces — so a caller forwarding an unsanitized URL path
// segment cannot escape dir.
func Read(
	dir, fileName string,
) (data []byte, contentType string, err error) {
	if fileName == "" || fileName != filepath.Base(fileName) {
		return nil, "", ErrNotFound
	}
	path := filepath.Join(dir, fileName)
	info, statErr := os.Stat(path)
	if statErr != nil || info.Size() > MaxBytes {
		return nil, "", ErrNotFound
	}
	//nolint:gosec // G304: dir is chat-scoped and already resolved by the caller; fileName is verified above to carry no separators.
	f, err := os.Open(path)
	if err != nil {
		return nil, "", ErrNotFound
	}
	defer func() { _ = f.Close() }()
	data, err = io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || int64(len(data)) > MaxBytes {
		return nil, "", ErrNotFound
	}
	return data, ContentType(data), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/repositories/chat/attachments/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/attachments/
git commit -m "feat(attachments): add durable-store repository (size cap, naming, sniffing)"
```

---

### Task 3: Chat usecase — `UploadAttachment` / `ReadAttachment`

**Files:**
- Create: `api/internal/app/usecases/chat/internal/turn/attachments.go`
- Create: `api/internal/app/usecases/chat/attachments.go`
- Modify: `api/internal/app/usecases/chat/turn.go:21-96` (add 2 methods to `TurnUsecase` interface)
- Test: `api/internal/app/usecases/chat/attachments_test.go`

**Interfaces:**
- Consumes: `worktreepath.AttachmentsDir` (Task 1), `repoattachments.Store`/`Read`/`ErrNotFound` (Task 2), `t.chats.GetChat`, `t.ws.AgentChatsDir` (existing `Turns` fields)
- Produces:
  - `type UploadAttachmentInput struct { ID, OriginalName string; Data []byte }` (alias `chat.UploadAttachmentInput = turn.UploadAttachmentInput`)
  - `type StoredAttachment struct { Ref, FileName, ContentType string; Size int }` (alias `chat.StoredAttachment = turn.StoredAttachment`)
  - `func (u *Usecase) UploadAttachment(ctx, chatID string, in UploadAttachmentInput) (StoredAttachment, error)`
  - `func (u *Usecase) ReadAttachment(ctx, chatID, fileName string) ([]byte, string, error)`

- [ ] **Step 1: Write the failing test**

```go
package chat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
}

func TestUploadAttachment_StoresAndReturnsTheDurableRef(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.NoError(t, err)
	assert.Equal(t, "chats/"+chatID+"/attachments/ab12-photo.png", stored.Ref)
	assert.Equal(t, "ab12-photo.png", stored.FileName)
	assert.Equal(t, "image/png", stored.ContentType)
	assert.Equal(t, len(pngBytes()), stored.Size)
}

func TestUploadAttachment_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.UploadAttachment(f.ctx, "no-such-chat", agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("x"),
	})
	require.Error(t, err)
}

func TestReadAttachment_RoundTripsAnUploadedFile(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.NoError(t, err)

	data, contentType, err := f.usecase.ReadAttachment(f.ctx, chatID, stored.FileName)
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
	assert.Equal(t, "image/png", contentType)
}

func TestReadAttachment_MissingFileIsNotFound(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, _, err := f.usecase.ReadAttachment(f.ctx, chatID, "no-such-file.png")
	assert.ErrorIs(t, err, repoattachments.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/... -run TestUploadAttachment -v`
Expected: FAIL — `f.usecase.UploadAttachment undefined`

- [ ] **Step 3: Write minimal implementation**

```go
// api/internal/app/usecases/chat/internal/turn/attachments.go
package turn

import (
	"context"
	"fmt"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// UploadAttachmentInput is one attachment's identity and bytes, already
// extracted from whichever of the handler's two ingestion shapes the request
// used — see chat/handlers.UploadAttachment.
type UploadAttachmentInput struct {
	// ID is the client-supplied shortid (nanoid); the backend never mints one
	// (design spec: it may need to match an Excalidraw fence-tag id).
	ID           string
	OriginalName string
	Data         []byte
}

// StoredAttachment is what a successful upload becomes: the durable logical
// reference the caller writes back into the message's markdown text, plus
// metadata a file card renders without a second fetch.
type StoredAttachment struct {
	Ref         string
	FileName    string
	Size        int
	ContentType string
}

// UploadAttachment stores one attachment into chatID's durable attachment
// directory and returns the logical reference the frontend encodes into
// ![]()/[]().
func (t *Turns) UploadAttachment(
	ctx context.Context,
	chatID string,
	in UploadAttachmentInput,
) (StoredAttachment, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: chat: %w", err)
	}
	chatsDir, err := t.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: chats dir: %w", err)
	}
	dir := worktreepath.AttachmentsDir(chatsDir, chatID)
	fileName, contentType, err := repoattachments.Store(dir, in.ID, in.OriginalName, in.Data)
	if err != nil {
		return StoredAttachment{}, fmt.Errorf("agent: upload attachment: %w", err)
	}
	return StoredAttachment{
		Ref: "chats/" + chatID + "/attachments/" + fileName, FileName: fileName,
		Size: len(in.Data), ContentType: contentType,
	}, nil
}

// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
// sniffed content type, or repoattachments.ErrNotFound.
func (t *Turns) ReadAttachment(
	ctx context.Context,
	chatID, fileName string,
) ([]byte, string, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: read attachment: chat: %w", err)
	}
	chatsDir, err := t.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: read attachment: chats dir: %w", err)
	}
	dir := worktreepath.AttachmentsDir(chatsDir, chatID)
	return repoattachments.Read(dir, fileName)
}
```

```go
// api/internal/app/usecases/chat/attachments.go
package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
)

type UploadAttachmentInput = turn.UploadAttachmentInput
type StoredAttachment = turn.StoredAttachment

// UploadAttachment stores one attachment into chatID's durable attachment
// store and returns the logical reference to write back into the message.
func (u *Usecase) UploadAttachment(
	ctx context.Context,
	chatID string,
	in UploadAttachmentInput,
) (StoredAttachment, error) {
	return u.turns.UploadAttachment(ctx, chatID, in)
}

// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
// sniffed content type.
func (u *Usecase) ReadAttachment(
	ctx context.Context,
	chatID, fileName string,
) ([]byte, string, error) {
	return u.turns.ReadAttachment(ctx, chatID, fileName)
}
```

In `turn.go`, add to the `TurnUsecase` interface (anywhere in the block, e.g. after `Telemetry`):

```go
	// UploadAttachment stores one attachment into chatID's durable attachment
	// store, returning the logical reference written back into the message.
	UploadAttachment(
		ctx context.Context,
		chatID string,
		in UploadAttachmentInput,
	) (StoredAttachment, error)

	// ReadAttachment resolves chatID's stored attachment fileName to bytes and
	// a sniffed content type, for the asset-serving GET endpoint.
	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/... -run 'TestUploadAttachment|TestReadAttachment' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/
git commit -m "feat(chat): add UploadAttachment/ReadAttachment usecase methods"
```

---

### Task 4: HTTP handlers, DTO, and route registration

**Files:**
- Create: `api/internal/api/v0/dto/agent_attachment.go`
- Create: `api/internal/api/v0/endpoints/chat/handlers/attachments.go`
- Modify: `api/internal/api/v0/endpoints/chat/handlers/handlers.go:59-96` (add 2 methods to `TurnUsecase`)
- Modify: `api/internal/api/v0/endpoints/chat/handlers/hooks_test.go` (extend `fakeAgentUsecase` struct)
- Modify: `api/internal/api/v0/endpoints/chat/routes.go:82-83` (register routes)
- Modify: `api/internal/api/libs/status.go:160-172` (map `attachments.ErrNotFound` → 404)
- Test: `api/internal/api/v0/endpoints/chat/handlers/attachments_test.go`

**Interfaces:**
- Consumes: `agentusecase.UploadAttachmentInput`/`StoredAttachment` (Task 3), `repoattachments.MaxBytes`/`ErrNotFound`/`SyntheticName`/`ContentType` (Task 2)
- Produces:
  - `POST .../workspaces/:wsId/chats/:id/attachments` → `dto.ChatAttachmentDTO`
  - `GET .../workspaces/:wsId/chats/:id/attachments/:file` → raw bytes, sniffed `Content-Type`
  - `dto.ChatAttachmentDTO{Ref, FileName, Size, ContentType}` (JSON keys: `ref`, `fileName`, `size`, `contentType`, wrapped in the standard `{"data": ...}` envelope by `libs.WriteQueryWithStatus`)

Note on a blank filename: the handler defensively synthesizes a name via `repoattachments.SyntheticName` when the client sends none, rather than requiring every caller to get this right — see Step 3's `readAttachmentFromMultipart`/`readAttachmentFromPath`. The frontend (Task 20) still sends a real name whenever it can; this is the backend's own backstop, not a substitute for that.

- [ ] **Step 1: Write the failing test**

```go
package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

func multipartAttachmentBody(t *testing.T, id, fileName string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	require.NoError(t, w.WriteField("id", id))
	fw, err := w.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf, w.FormDataContentType()
}

func TestUploadAttachment_MultipartStoresAndReturnsTheRef(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{
		uploadAttachmentOut: agentusecase.StoredAttachment{
			Ref: "chats/chat-1/attachments/ab12-photo.png", FileName: "ab12-photo.png",
			Size: 4, ContentType: "image/png",
		},
	})
	body, contentType := multipartAttachmentBody(t, "ab12", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var out struct {
		Data dto.ChatAttachmentDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "chats/chat-1/attachments/ab12-photo.png", out.Data.Ref)
	require.Len(t, uc.uploadAttachmentCalls, 1)
	assert.Equal(t, "ab12", uc.uploadAttachmentCalls[0].in.ID)
	assert.Equal(t, "photo.png", uc.uploadAttachmentCalls[0].in.OriginalName)
	assert.Equal(t, []byte("data"), uc.uploadAttachmentCalls[0].in.Data)
}

func TestUploadAttachment_MultipartRequiresAnID(t *testing.T) {
	body, contentType := multipartAttachmentBody(t, "", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUploadAttachment_MultipartSynthesizesANameWhenTheFilePartHasNone(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{
		uploadAttachmentOut: agentusecase.StoredAttachment{Ref: "chats/chat-1/attachments/ab12-pasted-image-x.png"},
	})
	body, contentType := multipartAttachmentBody(t, "ab12", "", pngBytesForTest())
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, uc.uploadAttachmentCalls, 1)
	assert.Regexp(t, `^pasted-image-.+\.png$`, uc.uploadAttachmentCalls[0].in.OriginalName)
}

func pngBytesForTest() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
}

func TestUploadAttachment_JSONPathVariantReadsTheHostFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/dropped.png"
	require.NoError(t, os.WriteFile(path, []byte("dropped bytes"), 0o600))

	uc := inWorkspace(&fakeAgentUsecase{
		uploadAttachmentOut: agentusecase.StoredAttachment{Ref: "chats/chat-1/attachments/ab12-dropped.png"},
	})
	reqBody := []byte(`{"path":"` + path + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, uc.uploadAttachmentCalls, 1)
	assert.Equal(t, "dropped.png", uc.uploadAttachmentCalls[0].in.OriginalName)
	assert.Equal(t, []byte("dropped bytes"), uc.uploadAttachmentCalls[0].in.Data)
}

func TestAttachment_ServesRawBytesWithTheSniffedContentType(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{
		readAttachmentData: []byte("raw bytes"), readAttachmentType: "text/plain; charset=utf-8",
	})
	ctx, rec := scoped(t, "/attachments/ab12-notes.txt")
	ctx.Params = append(ctx.Params, gin.Param{Key: "file", Value: "ab12-notes.txt"})

	newChatHandlers(uc).Attachment(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "raw bytes", rec.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
}

func TestAttachment_MissingFileIs404(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{readAttachmentErr: repoattachments.ErrNotFound})
	ctx, rec := scoped(t, "/attachments/no-such-file.png")
	ctx.Params = append(ctx.Params, gin.Param{Key: "file", Value: "no-such-file.png"})

	newChatHandlers(uc).Attachment(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/api/v0/endpoints/chat/handlers/... -run 'TestUploadAttachment|TestAttachment_' -v`
Expected: FAIL — `newChatHandlers(uc).UploadAttachment undefined`, `fakeAgentUsecase` has no field `uploadAttachmentOut`

- [ ] **Step 3: Write minimal implementation**

DTO:

```go
// api/internal/api/v0/dto/agent_attachment.go
package dto

// ChatAttachmentDTO is the upload endpoint's response: the durable logical
// reference the client encodes into the message's own markdown text, plus
// enough metadata to render a file card without a second fetch.
type ChatAttachmentDTO struct {
	Ref         string `json:"ref"`
	FileName    string `json:"fileName"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType"`
}
```

Handlers:

```go
// api/internal/api/v0/endpoints/chat/handlers/attachments.go
package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

// UploadAttachment handles POST .../workspaces/:wsId/chats/:id/attachments.
//
// Two ingestion shapes, the same precedent icons.ReadUpload already
// establishes (api/internal/api/v0/endpoints/icons/icons.go), adapted for
// attachments' own larger size cap and its own required "id" field:
//   - multipart/form-data: a "file" field (clipboard paste, browser file
//     picker — neither ever has a host path) plus an "id" form field.
//   - application/json: {"path": "...", "id": "..."} — a revived desktop
//     drag-and-drop, which yields a host path rather than bytes; the daemon
//     reads it itself. Same residual trust assumption icons.go's own
//     path-read variant documents: daemon and webview share a host, so this
//     is a user-chosen path from a native drop, not attacker-controlled.
//
// id is required on both shapes and never minted here — the frontend already
// generates a nanoid per attachment, and accepting rather than generating one
// is what lets that id double as an Excalidraw fence-tag id.
func (h *Handlers) UploadAttachment(c *gin.Context) {
	chat, ok := h.requireChatInWorkspace(c, c.Param("id"))
	if !ok {
		return
	}
	data, id, originalName, ok := readAttachmentUpload(c)
	if !ok {
		return
	}
	stored, err := h.turns.UploadAttachment(c.Request.Context(), chat.ID, agentusecase.UploadAttachmentInput{
		ID: id, OriginalName: originalName, Data: data,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusCreated, dto.ChatAttachmentDTO{
		Ref: stored.Ref, FileName: stored.FileName, Size: stored.Size, ContentType: stored.ContentType,
	})
}

func readAttachmentUpload(c *gin.Context) (data []byte, id, originalName string, ok bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return readAttachmentFromPath(c)
	}
	return readAttachmentFromMultipart(c)
}

func readAttachmentFromMultipart(c *gin.Context) ([]byte, string, string, bool) {
	id := c.Request.FormValue("id")
	if id == "" {
		libs.WriteErr(c, http.StatusBadRequest, "id required")
		return nil, "", "", false
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "file field required")
		return nil, "", "", false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, repoattachments.MaxBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "read error")
		return nil, "", "", false
	}
	if int64(len(data)) > repoattachments.MaxBytes {
		libs.WriteErr(c, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	originalName := header.Filename
	if originalName == "" {
		// Defensive backstop for a filename-less multipart part (some
		// clipboard-paste code paths never set one) — the client should
		// still send a real name whenever it can (Task 20).
		originalName = repoattachments.SyntheticName(repoattachments.ContentType(data), func() string {
			return time.Now().UTC().Format("20060102T150405")
		})
	}
	return data, id, originalName, true
}

// readAttachmentFromPath reads the attachment from an absolute host path
// supplied as JSON — see the residual trust assumption in UploadAttachment's
// own doc comment.
func readAttachmentFromPath(c *gin.Context) ([]byte, string, string, bool) {
	var body struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" || body.ID == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path and id required")
		return nil, "", "", false
	}
	info, err := os.Stat(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read attachment file")
		return nil, "", "", false
	}
	if info.Size() > repoattachments.MaxBytes {
		libs.WriteErr(c, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	//nolint:gosec // G304: path is an absolute host path from a native file dialog/drop, the same residual trust model as icons.go's readFromPath (daemon and webview share a host).
	f, err := os.Open(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read attachment file")
		return nil, "", "", false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, repoattachments.MaxBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "read error")
		return nil, "", "", false
	}
	if int64(len(data)) > repoattachments.MaxBytes {
		libs.WriteErr(c, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	return data, body.ID, filepath.Base(body.Path), true
}

// Attachment handles GET .../workspaces/:wsId/chats/:id/attachments/:file,
// serving one durable attachment's raw bytes with a sniffed Content-Type — the
// asset-serving endpoint the frontend's MarkdownAssetContext resolver calls to
// turn a stored ![]()/[]() ref into fetchable pixels (design spec,
// "Rendering in the composer & transcript"). Unlike
// .../activity/:toolId/payload (its closest shape precedent), this endpoint
// sniffs the real Content-Type rather than hardcoding text/plain, since an
// attachment is exactly as likely to be an image or a PDF as text.
func (h *Handlers) Attachment(c *gin.Context) {
	chat, ok := h.requireChatInWorkspace(c, c.Param("id"))
	if !ok {
		return
	}
	data, contentType, err := h.turns.ReadAttachment(c.Request.Context(), chat.ID, c.Param("file"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, contentType, data)
}
```

`handlers.go` — add to `TurnUsecase` interface:

```go
	UploadAttachment(
		ctx context.Context,
		chatID string,
		in agentusecase.UploadAttachmentInput,
	) (agentusecase.StoredAttachment, error)

	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)
```

`hooks_test.go` — add fields to `fakeAgentUsecase` (near `payloadCalls`):

```go
	uploadAttachmentCalls []uploadAttachmentCall
	uploadAttachmentOut   agentusecase.StoredAttachment
	uploadAttachmentErr   error

	readAttachmentCalls []readAttachmentCall
	readAttachmentData  []byte
	readAttachmentType  string
	readAttachmentErr   error
```

New test file adds the matching methods/types (any file in `package handlers_test` may define new methods on `fakeAgentUsecase`):

```go
// in attachments_test.go
type uploadAttachmentCall struct {
	chatID string
	in     agentusecase.UploadAttachmentInput
}
type readAttachmentCall struct{ chatID, fileName string }

func (f *fakeAgentUsecase) UploadAttachment(
	_ context.Context, chatID string, in agentusecase.UploadAttachmentInput,
) (agentusecase.StoredAttachment, error) {
	f.uploadAttachmentCalls = append(f.uploadAttachmentCalls, uploadAttachmentCall{chatID: chatID, in: in})
	return f.uploadAttachmentOut, f.uploadAttachmentErr
}

func (f *fakeAgentUsecase) ReadAttachment(
	_ context.Context, chatID, fileName string,
) ([]byte, string, error) {
	f.readAttachmentCalls = append(f.readAttachmentCalls, readAttachmentCall{chatID: chatID, fileName: fileName})
	return f.readAttachmentData, f.readAttachmentType, f.readAttachmentErr
}
```

`routes.go` — after the `activity/:toolId/payload` line:

```go
	wsScoped.POST("/chats/:id/attachments", h.UploadAttachment)
	wsScoped.GET("/chats/:id/attachments/:file", h.Attachment)
```

`libs/status.go` — add import `repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"` and extend `isNotFound`:

```go
func isNotFound(err error) bool {
	return errors.Is(err, apperr.ErrNotFound) ||
		errors.Is(err, engineterminal.ErrSessionNotFound) ||
		errors.Is(err, asynxmodels.ErrNotFound) ||
		errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, project.ErrFolderNotFound) ||
		errors.Is(err, enginegit.ErrBranchNotFound) ||
		errors.Is(err, agentchat.ErrNotFound) ||
		errors.Is(err, agentrunner.ErrNotFound) ||
		errors.Is(err, repoattachments.ErrNotFound)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/api/v0/endpoints/chat/handlers/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v0/dto/agent_attachment.go api/internal/api/v0/endpoints/chat/handlers/ api/internal/api/v0/endpoints/chat/routes.go api/internal/api/libs/status.go
git commit -m "feat(chat): add attachment upload and asset-serving endpoints"
```

---

### Task 5: Attachment-reference scanner and dispatch-text rewriter

**Files:**
- Create: `api/internal/app/usecases/chat/internal/runner/dispatch_attachments.go`
- Test: `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_internal_test.go`

**Interfaces:**
- Consumes: `worktreepath.AttachmentsDir`/`AttachmentScratchDir`/`AttachmentScratchDirName` (Task 1), `repoattachments.Read` (Task 2)
- Produces: `func materializeAttachmentsForDispatch(chatsDir, worktree, chatID, runnerID, text string) (string, error)` (package-internal, consumed by Task 6 and Task 8)

- [ ] **Step 1: Write the failing test**

```go
package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

func TestMaterializeAttachmentsForDispatch_RewritesAReferencedFile(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	text := "please look at ![a photo](chats/chat-1/attachments/" + fileName + ") thanks"
	out, err := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, err)

	wantRel := ".crowbar-attachments/runner-1/" + fileName
	assert.Contains(t, out, wantRel)
	assert.NotContains(t, out, "chats/chat-1/attachments/"+fileName)
	data, err := os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName))
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(data))
}

func TestMaterializeAttachmentsForDispatch_LeavesUnreferencedTextUntouched(t *testing.T) {
	out, err := materializeAttachmentsForDispatch("chats", "worktree", "chat-1", "runner-1", "plain text, no attachments")
	require.NoError(t, err)
	assert.Equal(t, "plain text, no attachments", out)
}

func TestMaterializeAttachmentsForDispatch_IgnoresAReferenceToADifferentChat(t *testing.T) {
	text := "![x](chats/OTHER-CHAT/attachments/f.png)"
	out, err := materializeAttachmentsForDispatch("chats", "worktree", "chat-1", "runner-1", text)
	require.NoError(t, err)
	assert.Equal(t, text, out, "a reference naming a different chat's store must never be rewritten")
}

func TestMaterializeAttachmentsForDispatch_LeavesAMissingDurableFileUnrewritten(t *testing.T) {
	text := "![gone](chats/chat-1/attachments/never-uploaded.png)"
	out, err := materializeAttachmentsForDispatch(t.TempDir(), t.TempDir(), "chat-1", "runner-1", text)
	require.NoError(t, err)
	assert.Equal(t, text, out)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestMaterializeAttachmentsForDispatch -v`
Expected: FAIL — `undefined: materializeAttachmentsForDispatch`

- [ ] **Step 3: Write minimal implementation**

```go
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// attachmentRefPattern matches a markdown link/image target pointing at the
// durable attachment store: ![alt](chats/<chatID>/attachments/<file>) or
// [name](chats/<chatID>/attachments/<file>). Attachment filenames are always
// server-generated (attachments.Store strips whitespace/parens/brackets out
// of the original name), so a bare ")"-delimited capture is safe.
var attachmentRefPattern = regexp.MustCompile(`\]\((chats/([^/\s)]+)/attachments/([^)\s]+))\)`)

type attachmentRef struct {
	logical  string
	fileName string
}

// findAttachmentRefs returns every durable attachment reference in text that
// belongs to chatID. A reference naming a DIFFERENT chat is ignored — it
// names a directory this dispatch has no business reading from.
func findAttachmentRefs(chatID, text string) []attachmentRef {
	var out []attachmentRef
	for _, m := range attachmentRefPattern.FindAllStringSubmatch(text, -1) {
		if m[2] != chatID {
			continue
		}
		out = append(out, attachmentRef{logical: m[1], fileName: m[3]})
	}
	return out
}

// materializeAttachmentsForDispatch copies every attachment text references
// (belonging to chatID) from the durable store (chatsDir/<chatID>/attachments)
// into runnerID's scratch directory inside worktree, and returns a COPY of
// text with each durable reference rewritten to the scratch path. text is
// never mutated — the stored LedgerTurn.Text stays the durable reference
// forever; only this dispatch-time copy changes.
//
// A reference to a file no longer in the durable store (deleted out of band)
// is left unrewritten: the CLI then hits a plain "no such file" reading a
// path that does not resolve, rather than this silently sending a broken
// prompt.
func materializeAttachmentsForDispatch(
	chatsDir, worktree, chatID, runnerID, text string,
) (string, error) {
	refs := findAttachmentRefs(chatID, text)
	if len(refs) == 0 {
		return text, nil
	}
	scratchDir := worktreepath.AttachmentScratchDir(worktree, runnerID)
	//nolint:gosec // G301: 0o700 matches RunnerDir's own perms for daemon-managed chat state.
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return "", fmt.Errorf("agent: materialize attachments: mkdir scratch dir: %w", err)
	}
	durableDir := worktreepath.AttachmentsDir(chatsDir, chatID)
	out := text
	for _, ref := range refs {
		data, _, err := repoattachments.Read(durableDir, ref.fileName)
		if err != nil {
			continue
		}
		dest := filepath.Join(scratchDir, ref.fileName)
		//nolint:gosec // G306: read back only by the CLI subprocess this daemon just forked; matches the durable store's own perms.
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			return "", fmt.Errorf("agent: materialize attachments: write scratch copy: %w", err)
		}
		rel := filepath.ToSlash(filepath.Join(worktreepath.AttachmentScratchDirName, runnerID, ref.fileName))
		out = strings.ReplaceAll(out, ref.logical, rel)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestMaterializeAttachmentsForDispatch -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/dispatch_attachments.go api/internal/app/usecases/chat/internal/runner/dispatch_attachments_internal_test.go
git commit -m "feat(runner): scan and rewrite attachment references at dispatch time"
```

---

### Task 6: Wire materialization into `spawnRunner` + normal-exit cleanup

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/spawnplan.go:21-72` (`spawnPaths` struct + func)
- Modify: `api/internal/app/usecases/chat/internal/runner/spawn.go:63-238,414-430` (`spawnRunner`, `forkCLI`, `onRunnerExit`)
- Test: `api/internal/app/usecases/chat/spawn_attachments_test.go`

**Interfaces:**
- Consumes: `materializeAttachmentsForDispatch` (Task 5), `worktreepath.AttachmentScratchDir`/`RemoveUnderWorktree` (Task 1), `agentusecase.UploadAttachmentInput` (Task 3)
- Produces: no new exported symbols — this is the wiring that makes attachment refs actually reach the CLI and get cleaned up. `f.usecase.SubmitPrompt`'s existing return (`domain.AgentPromptSubmission{RunnerID, TerminalSessionID}`) is what a caller uses to locate the scratch dir if needed.

- [ ] **Step 1: Write the failing test**

```go
package chat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

func TestSubmitPrompt_MaterializesAttachmentsAndRewritesTheDispatchedMessage(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)

	text := "check this out ![photo](" + stored.Ref + ")"
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString())
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	wantText := "check this out ![photo](.crowbar-attachments/" + submission.RunnerID + "/" + stored.FileName + ")"
	assert.Equal(t, -1, indexOf(last.argv, text), "the raw durable ref must never reach the CLI's argv")
	assert.NotEqual(t, -1, indexOf(last.argv, wantText))

	data, err := os.ReadFile(filepath.Join(f.ws.worktree,
		worktreepath.AttachmentScratchDirName, submission.RunnerID, stored.FileName))
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(data))
}

func TestSubmitPrompt_CleansUpTheScratchCopyOnNormalExit(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, "![photo]("+stored.Ref+")", uuid.NewString())
	require.NoError(t, err)
	scratchDir := worktreepath.AttachmentScratchDir(f.ws.worktree, submission.RunnerID)
	require.DirExists(t, scratchDir)

	f.term.exit(t, submission.TerminalSessionID)

	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr), "scratch attachment dir must be removed on normal exit")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/... -run TestSubmitPrompt_Materializes -v`
Expected: FAIL — the rewritten path is absent from `argv`; scratch file is never written

- [ ] **Step 3: Write minimal implementation**

`spawnplan.go` — add `chatsDir` to `spawnPaths`:

```go
type spawnPaths struct {
	crowbarHome string
	projectID   string
	repoID      string
	worktree    string
	tmpDir      string
	chatsDir    string
}
```

and, in `spawnPaths`'s return (the `chatsDir` local variable already exists at line 42):

```go
	return spawnPaths{
		crowbarHome: crowbarHome,
		projectID:   projectID,
		repoID:      repoID,
		worktree:    worktree,
		tmpDir:      tmpDir,
		chatsDir:    chatsDir,
	}, nil
```

`spawn.go` — in `spawnRunner`, right after `worktree, tmpDir := paths.worktree, paths.tmpDir`:

```go
	dispatchMessage, err := materializeAttachmentsForDispatch(
		paths.chatsDir, worktree, chatID, runnerID, promptMessage,
	)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: materialize attachments: %w", err)
	}
```

then in the `renderSpawnContext` call, change `promptMessage: promptMessage,` to `promptMessage: dispatchMessage,`.

Defensive cleanup on the one abort path between materialization and fork (`SpawnPlan` render failure):

```go
	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		rs.agents.ForgetRunner(runnerID)
		worktreepath.RemoveUnderWorktree(ctx, worktree, worktreepath.AttachmentScratchDir(worktree, runnerID))
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}
```

`forkCLI`'s failure branch — add one line after the existing `RemoveUnderHome` call:

```go
	rs.pendingHooks.Discard(req.runnerID)
	rs.agents.ForgetRunner(req.runnerID)
	worktreepath.RemoveUnderHome(ctx, req.crowbarHome, req.tmpDir)
	worktreepath.RemoveUnderWorktree(ctx, req.worktree, worktreepath.AttachmentScratchDir(req.worktree, req.runnerID))
```

`onRunnerExit` — thread `worktree` through and clean the scratch dir on every normal exit:

```go
func (rs *Runners) onRunnerExit(home, worktree, runnerID, tmpDir string) func() {
	return func() {
		worktreepath.RemoveUnderHome(context.Background(), home, tmpDir)
		worktreepath.RemoveUnderWorktree(context.Background(), worktree,
			worktreepath.AttachmentScratchDir(worktree, runnerID))
		rs.apiConns.drop(runnerID)
		if rs.pendingHooks.MarkExited(runnerID) {
			return
		}
		rs.reconcileRunnerExit(context.Background(), runnerID)
	}
}
```

and its call site in `forkCLI`:

```go
	termSessID, err := rs.term.CreateCommand(ctx, req.workspaceID, req.worktree, req.argv, req.env,
		rs.onRunnerExit(req.crowbarHome, req.worktree, req.runnerID, req.tmpDir))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/... -run TestSubmitPrompt_Materializes -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/spawn.go api/internal/app/usecases/chat/internal/runner/spawnplan.go api/internal/app/usecases/chat/spawn_attachments_test.go
git commit -m "feat(runner): materialize attachments into the worktree at dispatch, clean up on exit"
```

---

### Task 7: Boot-time orphan reap for scratch attachments

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/lifecycle.go:112-165,239-256`
- Test: `api/internal/app/usecases/chat/boot_reconcile_attachments_test.go`

**Interfaces:**
- Consumes: `worktreepath.AttachmentScratchDir`/`RemoveUnderWorktree` (Task 1), `rs.ws.WorktreeDir` (existing)
- Produces: no new exported symbols — closes the dual-cleanup mechanism the design spec requires (mirrors `reapCrashOrphanRunnerTmp`)

- [ ] **Step 1: Write the failing test**

```go
package chat_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

func TestReconcileRunnersOnBoot_ReapsOrphanedScratchAttachments(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, "![photo]("+stored.Ref+")", uuid.NewString())
	require.NoError(t, err)
	scratchDir := worktreepath.AttachmentScratchDir(f.ws.worktree, submission.RunnerID)
	require.DirExists(t, scratchDir)

	// The daemon restarts: every PTY dies with no onExit callback ever firing.
	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr), "boot reconciliation must reap an orphaned scratch attachment dir")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/... -run TestReconcileRunnersOnBoot_ReapsOrphanedScratchAttachments -v`
Expected: FAIL — scratch dir still present after `ReconcileRunnersOnBoot`

- [ ] **Step 3: Write minimal implementation**

In `ReconcileRunnersOnBoot`'s loop, next to the existing tmp reap:

```go
		rs.reapCrashOrphanRunnerTmp(ctx, r)
		rs.reapCrashOrphanRunnerAttachments(ctx, r)
```

New function, placed right after `reapCrashOrphanRunnerTmp`:

```go
// reapCrashOrphanRunnerAttachments removes r's scratch attachment directory
// (if any) after a crash — the dual-mechanism counterpart to onRunnerExit's
// normal-exit cleanup, mirroring reapCrashOrphanRunnerTmp's own structure.
// RunnerDir's reap checks paths against crowbarHome; a scratch attachment
// copy lives INSIDE the worktree instead, so this checks against the
// worktree root via RemoveUnderWorktree, not RemoveUnderHome.
func (rs *Runners) reapCrashOrphanRunnerAttachments(
	ctx context.Context,
	runner agents.Runner,
) {
	_, _, _, worktree, err := rs.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner attachments: worktree dir (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	worktreepath.RemoveUnderWorktree(ctx, worktree, worktreepath.AttachmentScratchDir(worktree, runner.ID))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/... -run TestReconcileRunnersOnBoot_ReapsOrphanedScratchAttachments -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/lifecycle.go api/internal/app/usecases/chat/boot_reconcile_attachments_test.go
git commit -m "feat(runner): reap orphaned scratch attachment dirs on boot reconciliation"
```

---

### Task 8: Materialize attachments on the live API-push prompt path

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/runner/prompts.go:164-199` (`submitPromptOverAPI`)
- Test: `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_api_internal_test.go`

**Interfaces:**
- Consumes: `materializeAttachmentsForDispatch` (Task 5)
- Produces: `func (rs *Runners) rewritePromptTextForDispatch(ctx, workspaceID, chatID, worktree, runnerID, text string) (string, error)` (package-internal)

Context: `SubmitPrompt` has two delivery paths — the restart_tui path through `spawnRunner` (Task 6, hooks-only providers, the common case) and the mixed-transport live push through `submitPromptOverAPI`/`pushPromptOverAPI` for a provider already holding an open api-transport connection (no process respawn). Both are "outbound prompt text reaching a CLI whose cwd is the worktree" and both need the same rewrite. This path reuses the runner that was already spawned earlier in the chat's lifetime, so its scratch dir (keyed by that runner's id) is cleaned up by the SAME dual mechanism Task 6/7 already wired — not per-turn here, but for that runner's whole remaining lifetime. Disclosed, not hidden: a long mixed-transport session that references several attachments across many turns keeps all of them materialized until that CLI process itself exits, which the existing cleanup still bounds.

- [ ] **Step 1: Write the failing test**

```go
package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

type fakeWSReader struct{ chatsDir, worktree string }

func (f fakeWSReader) WorktreeDir(context.Context, string) (string, string, string, string, error) {
	return "", "", "", f.worktree, nil
}
func (f fakeWSReader) AgentChatsDir(context.Context, string) (string, error) {
	return f.chatsDir, nil
}

var _ seam.WorkspaceReader = fakeWSReader{}

func TestRewritePromptTextForDispatch_RewritesAnAttachmentReference(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	rs := &Runners{ws: fakeWSReader{chatsDir: chatsDir, worktree: worktree}}
	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	out, err := rs.rewritePromptTextForDispatch(context.Background(), "ws-1", "chat-1", worktree, "runner-1", text)
	require.NoError(t, err)

	assert.Contains(t, out, ".crowbar-attachments/runner-1/"+fileName)
	data, err := os.ReadFile(filepath.Join(worktree, ".crowbar-attachments", "runner-1", fileName))
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(data))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestRewritePromptTextForDispatch -v`
Expected: FAIL — `undefined: (*Runners).rewritePromptTextForDispatch`

- [ ] **Step 3: Write minimal implementation**

New helper in `prompts.go` (or `dispatch_attachments.go`):

```go
// rewritePromptTextForDispatch is materializeAttachmentsForDispatch with the
// chatsDir lookup folded in, for call sites that only have a workspaceID —
// both submitPromptOverAPI (this file) and spawnRunner (spawn.go, via
// paths.chatsDir already resolved) end up calling the same underlying scan.
func (rs *Runners) rewritePromptTextForDispatch(
	ctx context.Context,
	workspaceID, chatID, worktree, runnerID, text string,
) (string, error) {
	chatsDir, err := rs.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	return materializeAttachmentsForDispatch(chatsDir, worktree, chatID, runnerID, text)
}
```

`submitPromptOverAPI` — insert before `pushPromptOverAPI`:

```go
	dispatchText, err := rs.rewritePromptTextForDispatch(ctx, chat.WorkspaceID, chat.ID, worktree, live.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf(
			"agent: submit prompt: materialize attachments: %w", err,
		)
	}

	_, _, pushErr := rs.pushPromptOverAPI(ctx, live.ID, live.CurrentSession, worktree, dispatchText)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/chat/internal/runner/... -run TestRewritePromptTextForDispatch -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/chat/internal/runner/prompts.go api/internal/app/usecases/chat/internal/runner/dispatch_attachments_api_internal_test.go
git commit -m "feat(runner): materialize attachments on the live API-push prompt path too"
```

---

**Backend API summary (for Phase 3's client glue, Task 20):**

- `POST /v0/.../workspaces/:wsId/chats/:id/attachments` — multipart (`file` field + `id` field) or JSON `{"path", "id"}`. Response envelope: `{"data": {"ref": "chats/<chatId>/attachments/<storedFileName>", "fileName": "<storedFileName>", "size": <bytes>, "contentType": "<sniffed mime>"}}`. `413` over 25 MiB, `400` on missing `id`/`file`/`path`, `409` on an `id` collision within the same chat.
- `GET /v0/.../workspaces/:wsId/chats/:id/attachments/:file` — raw bytes, sniffed `Content-Type`, `404` if missing.
- Both routes sit under the same `wsScoped` router group as every other chat endpoint (workspace-checked, 404 if `:id` isn't in `:wsId`) — reached through the app's real hierarchical route builder (`chatBase(wsId)` on the frontend), not a flat path.

# Phase 2: Frontend — Rendering & Kind Resolvers

**Key design decision:** `MarkdownAssetContext.Provider` is wired **once**, at `AgentChatView` (the common ancestor of all three render sites: composer, interactive transcript message, static transcript message), not inside each leaf. Those three files need **zero code changes** — they already consume `chatComposerPlugins`/`chatComposerPluginsStatic`, and `MarkdownImageElement` already calls `useMarkdownAsset()` internally. Context is exactly the tool for "wire once, consume anywhere below," matching `MarkdownEditorPane`'s own existing pattern.

**File Structure:**

Create:
- `web/src/features/agent/composer/plate/attachments/chat-asset-resolver.ts` — ref parsing, URL building, byte/metadata fetch
- `web/src/features/agent/composer/plate/attachments/chat-markdown-asset-provider.tsx` — thin `MarkdownAssetContext.Provider` wrapper keyed on `wsId`
- `web/src/features/agent/composer/plate/attachments/attachment-lang.ts` — `text-attachment:{id}` / `excalidraw:{id}` fence-tag parser
- `web/src/features/agent/composer/plate/attachments/excalidraw-scene.ts` — structural Excalidraw scene JSON validator
- `web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx` — pill + read-only inspect modal
- `web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx` — persisted-PNG preview (read-only)
- `web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx` — `ChatCodeBlockElement`, lang-branching code-block renderer
- `web/src/features/agent/composer/plate/attachments/chat-link-kit.tsx` — `ChatLinkKit`/`ChatLinkKitStatic`
- `web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx` — `ChatLinkElement`, file-card link override
- `web/src/features/agent/composer/plate/attachments/resolve-csv.ts` — CSV table-vs-file resolver + GFM table serializer

Modify:
- `web/src/features/editor/markdown/plate/markdown-asset.ts` — add optional `resolve` override to `MarkdownAssetInfo`/`loadLocalImage` (backward-compatible)
- `web/src/features/agent/api/agent-api.ts` — export the already-existing `chatBase` helper
- `web/src/features/agent/chat/agent-chat-view.tsx` — wrap all 3 return branches in `ChatMarkdownAssetProvider`
- `web/src/features/agent/composer/plate/chat-composer-plugins.ts` — swap in `ChatCodeBlockElement` and `ChatLinkKit`/`ChatLinkKitStatic`
- `web/package.json` — add `papaparse` + `@types/papaparse`

---

### Task 9: Pluggable asset resolution in `markdown-asset.ts`

**Files:**
- Modify: `web/src/features/editor/markdown/plate/markdown-asset.ts:1-22` (interface + `loadLocalImage`'s guard)
- Test: `web/src/__tests__/features/editor/markdown/plate/markdown-asset.test.ts`

**Interfaces:**
- Consumes: nothing new
- Produces: `MarkdownAssetInfo.resolve?: (src: string) => Promise<string | null>` — every later task's resolver plugs in here

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it, vi } from 'vitest'
import { loadLocalImage, type MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'

vi.mock('@/features/file-system/controllers/platform', () => ({
  readWorkspaceFile: vi.fn(),
}))

describe('loadLocalImage resolve override', () => {
  it('calls the override with the raw src, bypassing readWorkspaceFile', async () => {
    const resolve = vi.fn().mockResolvedValue('data:image/png;base64,AAAA')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '', resolve }

    const result = await loadLocalImage(asset, 'chats/c1/attachments/x.png')

    expect(result).toBe('data:image/png;base64,AAAA')
    expect(resolve).toHaveBeenCalledWith('chats/c1/attachments/x.png')
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('still returns null for a self-loading src without calling the override', async () => {
    const resolve = vi.fn()
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: '', resolve }

    expect(await loadLocalImage(asset, 'https://example.com/a.png')).toBeNull()
    expect(resolve).not.toHaveBeenCalled()
  })

  it('falls back to readWorkspaceFile when no override is set (pre-existing behaviour)', async () => {
    vi.mocked(readWorkspaceFile).mockResolvedValue('\x89PNG')
    const asset: MarkdownAssetInfo = { wsId: 'ws1', fileDir: 'docs' }

    const result = await loadLocalImage(asset, 'logo.png')

    expect(readWorkspaceFile).toHaveBeenCalledWith('ws1', 'docs/logo.png')
    expect(result).toMatch(/^data:image\/png;base64,/)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-asset.test.ts`
Expected: FAIL — `resolve` isn't a field on `MarkdownAssetInfo` and `loadLocalImage` never checks it, so the first test calls `readWorkspaceFile` instead (mocked to resolve `undefined`), producing a mismatch on the first assertion.

- [ ] **Step 3: Write minimal implementation**

```ts
export interface MarkdownAssetInfo {
  /** Workspace id the file belongs to (for `readWorkspaceFile`, or a `resolve`
   *  override that needs it). */
  wsId: string
  /** The file's own directory, workspace-relative (''=root). Unused when
   *  `resolve` is set. */
  fileDir: string
  /** Override the default fileDir-relative `readWorkspaceFile` resolution.
   *  When present, `loadLocalImage` calls this directly with the raw `src`
   *  instead — the chat asset context (`chat-asset-resolver.ts`) uses this to
   *  route through the attachment-serving endpoint, since a chat-attachment
   *  ref isn't a workspace-relative path. */
  resolve?: (src: string) => Promise<string | null>
}
```

And in `loadLocalImage`, right after the existing guard:

```ts
export async function loadLocalImage(
  asset: MarkdownAssetInfo | null,
  src: string,
): Promise<string | null> {
  if (!asset || !src || isSelfLoading(src)) return null
  if (asset.resolve) return asset.resolve(src)
  const path = resolveAssetPath(asset.fileDir, src)
  // ...unchanged from here
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-asset.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/markdown/plate/markdown-asset.ts web/src/__tests__/features/editor/markdown/plate/markdown-asset.test.ts
git commit -m "feat(markdown): let MarkdownAssetInfo override its resolution path

Backward-compatible — existing callers (the file editor) are unaffected."
```

---

### Task 10: `chat-asset-resolver.ts` — ref parsing, URL building, fetch

**Files:**
- Modify: `web/src/features/agent/api/agent-api.ts:9-11` (export `chatBase`)
- Create: `web/src/features/agent/composer/plate/attachments/chat-asset-resolver.ts`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/chat-asset-resolver.test.ts`

**Interfaces:**
- Consumes: `MarkdownAssetInfo` (Task 9), `chatBase(wsId): string`, `API_BASE` (`@/lib/api`)
- Produces: `parseChatAttachmentRef(ref): {chatId, filename} | null`, `chatAttachmentUrl(wsId, ref): string | null`, `fetchChatAttachmentDataUrl(wsId, ref, signal?): Promise<string|null>`, `fetchChatAttachmentMetadata(wsId, ref, signal?): Promise<{filename, size:number|null}|null>`, `chatMarkdownAssetInfo(wsId): MarkdownAssetInfo` — all later tasks depend on this

- [ ] **Step 1: Write the failing test**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  chatAttachmentUrl,
  chatMarkdownAssetInfo,
  fetchChatAttachmentDataUrl,
  fetchChatAttachmentMetadata,
  parseChatAttachmentRef,
} from '@/features/agent/composer/plate/attachments/chat-asset-resolver'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

describe('parseChatAttachmentRef', () => {
  it('extracts chatId and filename', () => {
    expect(parseChatAttachmentRef('chats/c1/attachments/pasted-image-1.png')).toEqual({
      chatId: 'c1',
      filename: 'pasted-image-1.png',
    })
  })

  it('rejects anything not exactly that shape', () => {
    expect(parseChatAttachmentRef('docs/readme.md')).toBeNull()
    expect(parseChatAttachmentRef('chats/c1/attachments/')).toBeNull()
    expect(parseChatAttachmentRef('https://example.com/x.png')).toBeNull()
  })
})

describe('chatAttachmentUrl', () => {
  it('builds the URL through the same chatBase every chat endpoint uses', () => {
    expect(chatAttachmentUrl('ws1', 'chats/c1/attachments/a b.png')).toBe(
      '/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1/attachments/a%20b.png',
    )
  })

  it('returns null for a non-attachment ref', () => {
    expect(chatAttachmentUrl('ws1', 'not-an-attachment')).toBeNull()
  })
})

describe('fetchChatAttachmentDataUrl', () => {
  it('fetches bytes and encodes them as a data: URL', async () => {
    const blob = new Blob(['hello'], { type: 'text/plain' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(blob, { status: 200 })))
    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toMatch(
      /^data:text\/plain;base64,/,
    )
    vi.unstubAllGlobals()
  })

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 404 })))
    expect(await fetchChatAttachmentDataUrl('ws1', 'chats/c1/attachments/x.txt')).toBeNull()
    vi.unstubAllGlobals()
  })

  it('never fetches for a ref that is not a chat attachment', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    expect(await fetchChatAttachmentDataUrl('ws1', 'nope')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })
})

describe('fetchChatAttachmentMetadata', () => {
  it('reads size from Content-Length via a HEAD request', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 200, headers: { 'content-length': '1234' } }))
    vi.stubGlobal('fetch', fetchMock)

    expect(await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf')).toEqual({
      filename: 'report.pdf',
      size: 1234,
    })
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'HEAD' })
    vi.unstubAllGlobals()
  })

  it('still returns the filename with a null size when Content-Length is absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 200 })))
    expect(await fetchChatAttachmentMetadata('ws1', 'chats/c1/attachments/report.pdf')).toEqual({
      filename: 'report.pdf',
      size: null,
    })
    vi.unstubAllGlobals()
  })
})

describe('chatMarkdownAssetInfo', () => {
  it('produces a resolve() that round-trips through fetchChatAttachmentDataUrl', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(blob, { status: 200 })))
    const asset = chatMarkdownAssetInfo('ws1')
    expect(await asset.resolve?.('chats/c1/attachments/x.png')).toMatch(
      /^data:image\/png;base64,/,
    )
    vi.unstubAllGlobals()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-asset-resolver.test.ts`
Expected: FAIL — module doesn't exist yet.

- [ ] **Step 3: Write minimal implementation**

In `agent-api.ts`, change:

```ts
function chatBase(wsId: string): string {
```

to:

```ts
export function chatBase(wsId: string): string {
```

New file:

```ts
import { chatBase } from '@/features/agent/api/agent-api'
import { API_BASE } from '@/lib/api'
import type { MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'

/** A chat-attachment logical reference, as stored in a message's markdown
 *  text: `chats/{chatId}/attachments/{filename}`. The chatId a ref names
 *  isn't necessarily the chat currently open — parsed straight off the
 *  string, which is always enough to build the serving URL. */
export interface ParsedChatAttachmentRef {
  chatId: string
  filename: string
}

const REF_PATTERN = /^chats\/([^/]+)\/attachments\/(.+)$/

export function parseChatAttachmentRef(ref: string): ParsedChatAttachmentRef | null {
  const match = REF_PATTERN.exec(ref)
  if (!match) return null
  const [, chatId, filename] = match
  if (!chatId || !filename) return null
  return { chatId, filename }
}

/** Built through `chatBase` — the same hierarchical, project/repo-scoped
 *  builder every other chat endpoint in `agent-api.ts` uses — not a flat
 *  `/workspaces/{wsId}/...` path. */
export function chatAttachmentUrl(wsId: string, ref: string): string | null {
  const parsed = parseChatAttachmentRef(ref)
  if (!parsed) return null
  return (
    `${API_BASE}${chatBase(wsId)}/${encodeURIComponent(parsed.chatId)}` +
    `/attachments/${encodeURIComponent(parsed.filename)}`
  )
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error ?? new Error('failed to read blob'))
    reader.readAsDataURL(blob)
  })
}

/** Same return contract as `loadLocalImage` (a data: URL, or null) — plugs
 *  straight into `MarkdownAssetInfo.resolve` with no adapter. */
export async function fetchChatAttachmentDataUrl(
  wsId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<string | null> {
  const url = chatAttachmentUrl(wsId, ref)
  if (!url) return null
  try {
    const response = await fetch(url, { signal })
    if (!response.ok) return null
    return await blobToDataUrl(await response.blob())
  } catch {
    return null
  }
}

export interface ChatAttachmentMetadata {
  filename: string
  /** null when the server didn't report Content-Length, or the request
   *  failed — the file card still renders without a size. */
  size: number | null
}

/** Metadata for a file card without downloading the body — a HEAD against
 *  the same URL `fetchChatAttachmentDataUrl` GETs. */
export async function fetchChatAttachmentMetadata(
  wsId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<ChatAttachmentMetadata | null> {
  const parsed = parseChatAttachmentRef(ref)
  const url = chatAttachmentUrl(wsId, ref)
  if (!parsed || !url) return null
  try {
    const response = await fetch(url, { method: 'HEAD', signal })
    const len = response.headers.get('content-length')
    return { filename: parsed.filename, size: len ? Number(len) : null }
  } catch {
    return { filename: parsed.filename, size: null }
  }
}

/** The `MarkdownAssetContext` value for chat: same shape the standalone
 *  markdown file editor provides, but `resolve` routes through the
 *  attachment-serving endpoint instead of a workspace file read. `fileDir`
 *  is unused here — every chat-attachment ref already carries its own
 *  chatId+filename. */
export function chatMarkdownAssetInfo(wsId: string): MarkdownAssetInfo {
  return { wsId, fileDir: '', resolve: (src) => fetchChatAttachmentDataUrl(wsId, src) }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-asset-resolver.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/api/agent-api.ts web/src/features/agent/composer/plate/attachments/chat-asset-resolver.ts web/src/__tests__/features/agent/composer/plate/attachments/chat-asset-resolver.test.ts
git commit -m "feat(chat): add the chat-attachment ref/URL/fetch resolver"
```

---

### Task 11: `ChatMarkdownAssetProvider` + wire into `AgentChatView`

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/chat-markdown-asset-provider.tsx`
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx:28` (import), `:710-716`, `:718-742`, `:744-828` (wrap all 3 return branches)
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/chat-markdown-asset-provider.test.tsx`

**Interfaces:**
- Consumes: `chatMarkdownAssetInfo` (Task 10), `MarkdownAssetContext` (Task 9's file)
- Produces: `<ChatMarkdownAssetProvider wsId={string}>` — the provider every chat render site now sits under

- [ ] **Step 1: Write the failing test**

```tsx
import { useEffect, useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'

recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })

function Probe() {
  const asset = useMarkdownAsset()
  const [resolved, setResolved] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    void asset?.resolve?.('chats/c1/attachments/x.png').then(setResolved)
  }, [asset])
  return <span data-testid="probe">{resolved === undefined ? 'pending' : String(resolved)}</span>
}

describe('ChatMarkdownAssetProvider', () => {
  it('provides an asset whose resolve() fetches through the chat-attachment endpoint', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(blob, { status: 200 })))

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <Probe />
      </ChatMarkdownAssetProvider>,
    )

    await waitFor(() =>
      expect(screen.getByTestId('probe').textContent).toMatch(/^data:image\/png;base64,/),
    )
    vi.unstubAllGlobals()
  })

  it('memoizes the value across re-renders with the same wsId', () => {
    const seen: unknown[] = []
    function CaptureRef() {
      seen.push(useMarkdownAsset())
      return null
    }
    const { rerender } = render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    rerender(
      <ChatMarkdownAssetProvider wsId="ws1">
        <CaptureRef />
      </ChatMarkdownAssetProvider>,
    )
    expect(seen[0]).toBe(seen[1])
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-markdown-asset-provider.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Write minimal implementation**

New file:

```tsx
'use client'

import { useMemo, type ReactNode } from 'react'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { chatMarkdownAssetInfo } from './chat-asset-resolver'

interface ChatMarkdownAssetProviderProps {
  wsId: string
  children: ReactNode
}

/**
 * Wires `MarkdownAssetContext` for chat, high enough to cover every render
 * site under `AgentChatView` (`ChatMarkdownEditor`, `MarkdownMessage`,
 * `MarkdownMessageStatic`) with one provider — mirrors `MarkdownEditorPane`'s
 * own pattern: build the asset info once, provide it once, let every
 * descendant node component read it via `useMarkdownAsset()`.
 */
export function ChatMarkdownAssetProvider({ wsId, children }: ChatMarkdownAssetProviderProps) {
  const value = useMemo(() => chatMarkdownAssetInfo(wsId), [wsId])
  return <MarkdownAssetContext.Provider value={value}>{children}</MarkdownAssetContext.Provider>
}
```

In `agent-chat-view.tsx`, add the import after the `CaretEdges` import:

```ts
import type { CaretEdges } from '@/features/agent/composer/plate/chat-markdown-editor'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
```

Wrap the `settling` branch:

```tsx
  if (settling) {
    return (
      <ChatMarkdownAssetProvider wsId={wsId}>
        <section className="agent-chat chat" aria-label="Agent chat">
          {transcript}
        </section>
      </ChatMarkdownAssetProvider>
    )
  }
```

Wrap the `blank` branch:

```tsx
  if (blank) {
    return (
      <ChatMarkdownAssetProvider wsId={wsId}>
        <section className="agent-chat chat" aria-label="Agent chat">
          <AgentEmptyDocument
            ref={emptyDocRef}
            draft={seed.text}
            draftSeed={seed.n}
            hasText={draft.trim().length > 0}
            onDraftChange={updateDraft}
            onSubmit={() => enqueueDraft()}
            onKeyDown={handleKeyDown}
            controls={selectionCluster}
            working={working}
            canStop={live}
            sending={prompts.deliveryPending}
            onStop={handleStop}
          />
          {composerError && (
            <p className="meta" role="alert">
              {composerError}
            </p>
          )}
        </section>
      </ChatMarkdownAssetProvider>
    )
  }
```

Wrap the default branch (open the provider before `<section`, close it after the matching `</section>`, keeping every inner line as-is — run the formatter afterward to fix indentation):

```tsx
  return (
    <ChatMarkdownAssetProvider wsId={wsId}>
    <section
      className="agent-chat chat"
      aria-label="Agent chat"
      style={
        {
          '--agent-dock-h': `${Math.round(dockHeight)}px`,
          '--agent-scrollbar-w': `${scrollbarWidth}px`,
        } as React.CSSProperties
      }
    >
      {/* ...unchanged... */}
    </section>
    </ChatMarkdownAssetProvider>
  )
}
```

Then run `cd web && bun run format` (prettier) to fix indentation throughout the wrapped block.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-markdown-asset-provider.test.tsx`
Expected: PASS. Also run the full existing suite to confirm no regression (its mocks don't consume `useMarkdownAsset`, so the wrap is invisible to it): `cd web && bunx vitest run src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
Expected: PASS, unchanged.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/chat-markdown-asset-provider.tsx web/src/features/agent/chat/agent-chat-view.tsx web/src/__tests__/features/agent/composer/plate/attachments/chat-markdown-asset-provider.test.tsx
git commit -m "feat(chat): wire MarkdownAssetContext into the chat surface"
```

---

### Task 12: Prove `![alt](ref)` resolves real bytes end-to-end

**Files:**
- Test only: `web/src/__tests__/features/agent/composer/plate/attachments/image-attachment-resolution.test.tsx`

**Interfaces:**
- Consumes: `ChatMarkdownAssetProvider` (Task 11), existing `MarkdownMessage`/`MarkdownMessageStatic`, existing `MarkdownImageKit`
- Produces: nothing new — this is the design spec's explicit "prove it resolves end-to-end" requirement, satisfied entirely by Tasks 9–11 plus the already-registered `MarkdownImageKit`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })

describe('chat image attachment resolution', () => {
  it('resolves ![alt](ref) to real fetched bytes in the static transcript render', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(new Blob(['x'], { type: 'image/png' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: 'a diagram' })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/chats/c1/attachments/shot.png'),
      expect.anything(),
    )
    vi.unstubAllGlobals()
  })

  it('resolves the same reference in the interactive editor', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response(new Blob(['x'], { type: 'image/png' }), { status: 200 })),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessage>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessage>
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: 'a diagram' })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    vi.unstubAllGlobals()
  })

  it('leaves the raw src alone with no MarkdownAssetContext (pre-existing behaviour, unchanged)', () => {
    render(<MarkdownMessageStatic>{'![a diagram](chats/c1/attachments/shot.png)'}</MarkdownMessageStatic>)
    expect(screen.getByRole('img', { name: 'a diagram' }).getAttribute('src')).toBe(
      'chats/c1/attachments/shot.png',
    )
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/image-attachment-resolution.test.tsx`
Expected: FAIL before Tasks 9–11 land (`img.src` stays the raw ref, never a `data:` URL). If run after Tasks 9–11 are already merged, this step is a no-op regression check — note that explicitly rather than re-deriving a false "fails" claim.

- [ ] **Step 3: Write minimal implementation**

None — Tasks 9–11 are the implementation. This task is pure verification.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/image-attachment-resolution.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/__tests__/features/agent/composer/plate/attachments/image-attachment-resolution.test.tsx
git commit -m "test(chat): prove image attachments resolve real bytes end-to-end"
```

---

### Task 13: Attachment fence-tag parser (`attachment-lang.ts`)

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/attachment-lang.ts`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/attachment-lang.test.ts`

**Interfaces:**
- Consumes: nothing
- Produces: `parseAttachmentLang(lang): {kind:'text-attachment'|'excalidraw', id:string} | null`, id shape `/^[A-Za-z0-9_-]{6,}$/` — the editor-UX task's id-minting MUST match this shape (a default `nanoid()` of length ≥6 already does)

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import { parseAttachmentLang } from '@/features/agent/composer/plate/attachments/attachment-lang'

describe('parseAttachmentLang', () => {
  it('parses a valid text-attachment tag', () => {
    expect(parseAttachmentLang('text-attachment:AbC123xy')).toEqual({
      kind: 'text-attachment',
      id: 'AbC123xy',
    })
  })

  it('parses a valid excalidraw tag', () => {
    expect(parseAttachmentLang('excalidraw:AbC123xy')).toEqual({ kind: 'excalidraw', id: 'AbC123xy' })
  })

  it('rejects a bare tag with no id — ordinary discussion of the feature must not be hijacked', () => {
    expect(parseAttachmentLang('text-attachment')).toBeNull()
    expect(parseAttachmentLang('excalidraw')).toBeNull()
  })

  it('rejects an id shorter than 6 characters', () => {
    expect(parseAttachmentLang('text-attachment:abc')).toBeNull()
  })

  it('rejects an id with characters outside [A-Za-z0-9_-]', () => {
    expect(parseAttachmentLang('text-attachment:abc def!')).toBeNull()
  })

  it('rejects an unrelated language tag', () => {
    expect(parseAttachmentLang('python')).toBeNull()
  })

  it('rejects null/undefined', () => {
    expect(parseAttachmentLang(null)).toBeNull()
    expect(parseAttachmentLang(undefined)).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/attachment-lang.test.ts`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Write minimal implementation**

```ts
export type AttachmentLangKind = 'text-attachment' | 'excalidraw'

export interface ParsedAttachmentLang {
  kind: AttachmentLangKind
  id: string
}

const ATTACHMENT_ID_PATTERN = /^[A-Za-z0-9_-]{6,}$/

/** Parses a code-block's `lang` (the fence's info string) into its attachment
 *  kind + id. A bare `text-attachment`/`excalidraw` tag with no id — or an id
 *  that's too short or has disallowed characters — deliberately does NOT
 *  match, so ordinary discussion of this feature (a fence in this repo's own
 *  docs, say) renders as a plain code block instead of being hijacked. */
export function parseAttachmentLang(lang: string | null | undefined): ParsedAttachmentLang | null {
  if (!lang) return null
  const colon = lang.indexOf(':')
  if (colon === -1) return null
  const kind = lang.slice(0, colon)
  const id = lang.slice(colon + 1)
  if (kind !== 'text-attachment' && kind !== 'excalidraw') return null
  if (!ATTACHMENT_ID_PATTERN.test(id)) return null
  return { kind, id }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/attachment-lang.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/attachment-lang.ts web/src/__tests__/features/agent/composer/plate/attachments/attachment-lang.test.ts
git commit -m "feat(chat): add the text-attachment/excalidraw fence-tag parser"
```

---

### Task 14: Excalidraw scene validator (`excalidraw-scene.ts`)

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/excalidraw-scene.ts`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/excalidraw-scene.test.ts`

**Interfaces:**
- Consumes: nothing
- Produces: `ParsedExcalidrawScene { elements: unknown[]; appState: Record<string, unknown> }`, `parseExcalidrawScene(raw: string): ParsedExcalidrawScene | null`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import { parseExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

describe('parseExcalidrawScene', () => {
  it('accepts a plausible scene shape', () => {
    const raw = JSON.stringify({
      elements: [{ type: 'rectangle' }],
      appState: { viewBackgroundColor: '#fff' },
    })
    expect(parseExcalidrawScene(raw)).toEqual({
      elements: [{ type: 'rectangle' }],
      appState: { viewBackgroundColor: '#fff' },
    })
  })

  it('rejects invalid JSON', () => {
    expect(parseExcalidrawScene('not json')).toBeNull()
  })

  it('rejects valid JSON missing elements/appState', () => {
    expect(parseExcalidrawScene(JSON.stringify({ foo: 'bar' }))).toBeNull()
  })

  it('rejects elements that is not an array', () => {
    expect(parseExcalidrawScene(JSON.stringify({ elements: 'nope', appState: {} }))).toBeNull()
  })

  it('rejects a JSON array or primitive at the top level', () => {
    expect(parseExcalidrawScene('[]')).toBeNull()
    expect(parseExcalidrawScene('42')).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/excalidraw-scene.test.ts`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Write minimal implementation**

```ts
export interface ParsedExcalidrawScene {
  elements: unknown[]
  appState: Record<string, unknown>
}

/** Structural check only — not the real Excalidraw type (that library isn't
 *  a dependency of this read-only preview renderer). Just enough to reject a
 *  fenced block that merely mentions "excalidraw" in ordinary prose, or valid
 *  JSON that isn't a scene, before treating it as a real diagram. */
export function parseExcalidrawScene(raw: string): ParsedExcalidrawScene | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null
  const obj = parsed as Record<string, unknown>
  if (!Array.isArray(obj.elements)) return null
  if (typeof obj.appState !== 'object' || obj.appState === null) return null
  return { elements: obj.elements, appState: obj.appState as Record<string, unknown> }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/excalidraw-scene.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/excalidraw-scene.ts web/src/__tests__/features/agent/composer/plate/attachments/excalidraw-scene.test.ts
git commit -m "feat(chat): add a structural Excalidraw scene validator"
```

---

### Task 15: Text-attachment pill + modal

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/text-attachment-pill.test.tsx`

**Interfaces:**
- Consumes: `AppDialog` (`@/components/ui/dialog`), `FileText` (`@phosphor-icons/react`)
- Produces: `TextAttachmentPill({ text: string })`

- [ ] **Step 1: Write the failing test**

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { TextAttachmentPill } from '@/features/agent/composer/plate/attachments/text-attachment-pill'

describe('TextAttachmentPill', () => {
  it('shows a line count and opens a modal with the raw text on click', () => {
    const text = 'line one\nline two\nline three'
    render(<TextAttachmentPill text={text} />)

    expect(screen.queryByTestId('text-attachment-raw')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))

    expect(screen.getByTestId('text-attachment-raw').textContent).toBe(text)
  })

  it('renders the text with no markdown interpretation', () => {
    const text = '# Not a heading\n**not bold**'
    render(<TextAttachmentPill text={text} />)
    fireEvent.click(screen.getByRole('button', { name: /pasted text/i }))
    expect(screen.getByTestId('text-attachment-raw').textContent).toBe(text)
    expect(screen.queryByRole('heading')).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/text-attachment-pill.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Write minimal implementation**

```tsx
'use client'

import { useState } from 'react'
import { FileText } from '@phosphor-icons/react'
import { AppDialog } from '@/components/ui/dialog'

interface TextAttachmentPillProps {
  text: string
}

/** Pasted-text attachment: a pill, and a read-only modal showing the raw
 *  text verbatim — no markdown rendering (per the design spec, "no markdown
 *  for now"). */
export function TextAttachmentPill({ text }: TextAttachmentPillProps) {
  const [open, setOpen] = useState(false)
  const lineCount = text.split('\n').length

  return (
    <>
      <button
        type="button"
        className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/50 px-2.5 py-1 text-xs text-muted-foreground hover:bg-muted"
        onClick={() => setOpen(true)}
      >
        <FileText size={14} />
        Pasted text · {lineCount} {lineCount === 1 ? 'line' : 'lines'}
      </button>
      {open && (
        <AppDialog
          title="Pasted text"
          icon={FileText}
          size="lg"
          onClose={() => setOpen(false)}
          classNames={{ content: 'overflow-auto p-4' }}
        >
          <pre data-testid="text-attachment-raw" className="whitespace-pre-wrap break-words font-mono text-xs">
            {text}
          </pre>
        </AppDialog>
      )}
    </>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/text-attachment-pill.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx web/src/__tests__/features/agent/composer/plate/attachments/text-attachment-pill.test.tsx
git commit -m "feat(chat): add the text-attachment pill and inspect modal"
```

---

### Task 16: Excalidraw preview (persisted PNG)

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/excalidraw-preview.test.tsx`

**Interfaces:**
- Consumes: `useMarkdownAsset`, `loadLocalImage` (Task 9), `ParsedExcalidrawScene` (Task 14), `ChatMarkdownAssetProvider` (Task 11, test only)
- Produces: `ExcalidrawPreview({ scene: ParsedExcalidrawScene; pngRef?: string })`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ExcalidrawPreview } from '@/features/agent/composer/plate/attachments/excalidraw-preview'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })

const scene = { elements: [{ type: 'rectangle' }, { type: 'ellipse' }], appState: {} }

describe('ExcalidrawPreview', () => {
  it('shows a placeholder with the element count while there is no PNG ref', () => {
    render(<ExcalidrawPreview scene={scene} />)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    expect(screen.getByText(/2 elements/i)).toBeInTheDocument()
  })

  it('resolves and shows the persisted PNG once a pngRef and asset context are present', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response(new Blob(['x'], { type: 'image/png' }), { status: 200 })),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    vi.unstubAllGlobals()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/excalidraw-preview.test.tsx`
Expected: FAIL — module doesn't exist.

- [ ] **Step 3: Write minimal implementation**

```tsx
'use client'

import { useEffect, useState } from 'react'
import { loadLocalImage, useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import type { ParsedExcalidrawScene } from './excalidraw-scene'

interface ExcalidrawPreviewProps {
  scene: ParsedExcalidrawScene
  /** The persisted PNG's ref — the sibling `![diagram](ref)` node's `url`,
   *  when there is one. Absent falls back to a placeholder. */
  pngRef?: string
}

/** A settled Excalidraw attachment's preview: the persisted PNG, not a
 *  re-render of the scene JSON (no Excalidraw library dependency — the
 *  embedded drawing editor that produces the JSON+PNG is a separate task).
 *  Resolves through the same `MarkdownAssetContext`/`loadLocalImage`
 *  `MarkdownImageElement` uses — same contract, no new fetch path. */
export function ExcalidrawPreview({ scene, pngRef }: ExcalidrawPreviewProps) {
  const asset = useMarkdownAsset()
  const [src, setSrc] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!asset || !pngRef) {
      setSrc(null)
      return
    }
    void loadLocalImage(asset, pngRef).then((data) => {
      if (!cancelled) setSrc(data)
    })
    return () => {
      cancelled = true
    }
  }, [asset, pngRef])

  return (
    <div className="excalidraw-preview rounded-md border border-border bg-background/60 p-2">
      {src ? (
        <img src={src} alt="Excalidraw diagram" className="max-w-full rounded" />
      ) : (
        <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
          Excalidraw diagram ({scene.elements.length} elements)
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/excalidraw-preview.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx web/src/__tests__/features/agent/composer/plate/attachments/excalidraw-preview.test.tsx
git commit -m "feat(chat): add the read-only Excalidraw diagram preview"
```

---

### Task 17: `ChatCodeBlockElement` + wire into `chat-composer-plugins.ts`

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx`
- Modify: `web/src/features/agent/composer/plate/chat-composer-plugins.ts:27-34, 90-94` (import + `node.component`)
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx`

**Interfaces:**
- Consumes: `parseAttachmentLang` (Task 13), `parseExcalidrawScene` (Task 14), `TextAttachmentPill` (Task 15), `ExcalidrawPreview` (Task 16)
- Produces: `ChatCodeBlockElement` registered as `CodeBlockPlugin`'s node component in both `chatComposerPlugins` and (derived) `chatComposerPluginsStatic` — editor-UX must insert `text-attachment`/`excalidraw` fences as real `code_block` nodes with `lang: 'text-attachment:{id}'` / `'excalidraw:{id}'`, and for Excalidraw, an `img` node with `url: <png ref>` immediately following in the same parent

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

describe('chat attachment code blocks', () => {
  it('renders a plain code block for a bare ```text-attachment fence with no id', () => {
    render(
      <MarkdownMessageStatic>{'```text-attachment\njust discussing the feature\n```'}</MarkdownMessageStatic>,
    )
    expect(screen.getByText('just discussing the feature')).toBeInTheDocument()
    expect(screen.queryByText(/pasted text/i)).toBeNull()
  })

  it('renders a pill for a fence with a valid id, keeping the raw text mounted but hidden', () => {
    render(
      <MarkdownMessageStatic>{'```text-attachment:AbC123xy\nsome long pasted text\n```'}</MarkdownMessageStatic>,
    )
    expect(screen.getByRole('button', { name: /pasted text/i })).toBeInTheDocument()
    expect(screen.getByText('some long pasted text').closest('.hidden')).not.toBeNull()
  })

  it('renders a plain code block for an excalidraw fence with content that is not valid scene JSON', () => {
    render(<MarkdownMessageStatic>{'```excalidraw:AbC123xy\nnot json\n```'}</MarkdownMessageStatic>)
    expect(screen.getByText('not json')).toBeInTheDocument()
  })

  it('renders a diagram preview for a valid excalidraw fence', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(<MarkdownMessageStatic>{`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\``}</MarkdownMessageStatic>)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx`
Expected: FAIL — `chatComposerPluginsStatic` still renders every code block plainly (`CommentCodeBlockElement`), so the pill/diagram assertions fail.

- [ ] **Step 3: Write minimal implementation**

New file:

```tsx
'use client'

import type { ReactNode } from 'react'
import { type TCodeBlockElement, NodeApi, PathApi } from 'platejs'
import { PlateElement, type PlateElementProps } from 'platejs/react'
import { parseAttachmentLang } from './attachment-lang'
import { parseExcalidrawScene } from './excalidraw-scene'
import { TextAttachmentPill } from './text-attachment-pill'
import { ExcalidrawPreview } from './excalidraw-preview'

/** A code block's full multi-line source, reconstructed from its `code_line`
 *  children — `NodeApi.string(element)` would concatenate lines with no
 *  separator. Same technique `mermaid-code-block.tsx`'s isMermaid branch
 *  already uses. */
function codeBlockSource(element: TCodeBlockElement): string {
  return element.children.map((line) => NodeApi.string(line)).join('\n')
}

/** The `url` of the `img` node immediately following this code block, if
 *  any — the Excalidraw kind's persisted-PNG sibling (fenced JSON, then
 *  `![diagram](ref)` right after, per the design spec). */
function findFollowingImageRef(props: PlateElementProps<TCodeBlockElement>): string | undefined {
  const path = props.editor.api.findPath(props.element)
  if (!path) return undefined
  const entry = props.editor.api.node(PathApi.next(path))
  if (!entry) return undefined
  const [node] = entry
  return node.type === 'img' && typeof node.url === 'string' ? node.url : undefined
}

/**
 * Chat's fenced-code-block renderer. A `text-attachment:{id}`/`excalidraw:{id}`
 * tag with a validly-shaped id (and, for Excalidraw, content that actually
 * parses as a scene) renders a kind-specific preview; anything else —
 * including a bare tag with no id, or invalid JSON — falls through to the
 * same plain rendering `CommentCodeBlockElement` (the component this
 * replaces) already used, verbatim.
 *
 * The raw block stays mounted whenever a preview renders over it — same
 * hidden-but-present technique `mermaid-code-block.tsx` uses — so Slate's
 * node<->DOM mapping is never disturbed.
 */
export function ChatCodeBlockElement(props: PlateElementProps<TCodeBlockElement>) {
  const { element } = props
  const parsed = parseAttachmentLang(element.lang)

  const codeBody = (
    <pre className="overflow-x-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed [tab-size:2]">
      <code>{props.children}</code>
    </pre>
  )

  let preview: ReactNode = null
  if (parsed?.kind === 'text-attachment') {
    preview = <TextAttachmentPill text={codeBlockSource(element)} />
  } else if (parsed?.kind === 'excalidraw') {
    const scene = parseExcalidrawScene(codeBlockSource(element))
    if (scene) preview = <ExcalidrawPreview scene={scene} pngRef={findFollowingImageRef(props)} />
  }

  return (
    <PlateElement {...props} className="my-2">
      {preview ? (
        <div className="chat-attachment-block">
          <div contentEditable={false} className="select-none">
            {preview}
          </div>
          <div className="hidden">{codeBody}</div>
        </div>
      ) : (
        codeBody
      )}
    </PlateElement>
  )
}
```

In `chat-composer-plugins.ts`:

```ts
import {
  CommentCodeLineElement,
  CommentTableCellElement,
  CommentTableCellHeaderElement,
  CommentTableElement,
  CommentTableRowElement,
} from '@/features/editor/markdown/plate/comment/comment-nodes'
import { ChatCodeBlockElement } from '@/features/agent/composer/plate/attachments/chat-code-block-node'
```

(drop `CommentCodeBlockElement` from that import — no longer used) and:

```ts
  CodeBlockPlugin.configure({
    inputRules: [CodeBlockRules.markdown({ on: 'match' })],
    node: { component: ChatCodeBlockElement },
    shortcuts: { toggle: { keys: 'mod+alt+8' } },
  }),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx`
Expected: PASS. Also re-run `web/src/__tests__/features/agent/transcript/plate/markdown-message.test.tsx` — its "keeps a fenced code block" test must still pass, since `ChatCodeBlockElement` falls through to identical plain rendering for an ordinary ```go block.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx web/src/features/agent/composer/plate/chat-composer-plugins.ts web/src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx
git commit -m "feat(chat): render text-attachment/excalidraw fences as pill/preview"
```

---

### Task 18: File-card link override + wire into `chat-composer-plugins.ts`

**Files:**
- Create: `web/src/features/agent/composer/plate/attachments/chat-link-kit.tsx`
- Create: `web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx`
- Modify: `web/src/features/agent/composer/plate/chat-composer-plugins.ts:10, 82, 115` (swap `LinkKit`/`LinkKitStatic` for `ChatLinkKit`/`ChatLinkKitStatic`)
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`

**Interfaces:**
- Consumes: `parseChatAttachmentRef`, `chatAttachmentUrl`, `fetchChatAttachmentMetadata` (Task 10), `useMarkdownAsset` (Task 9), `LinkElement`/`handleMarkdownAnchorClick` (existing), `FileExplorerIcon` (existing, `@/features/file-explorer/components/file-explorer-icon`)
- Produces: `ChatLinkElement`, `ChatLinkKit`/`ChatLinkKitStatic` registered — editor-UX inserts file attachments as an ordinary `a`/link node (`url: ref`, text children = filename); the card renders automatically whenever `ref` matches `chats/{chatId}/attachments/{filename}`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })

describe('chat attachment file card', () => {
  it('renders an ordinary link unchanged for a non-attachment href', () => {
    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>{'[docs](https://example.com)'}</MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )
    expect(screen.getByRole('link', { name: 'docs' })).toHaveAttribute('href', 'https://example.com')
  })

  it('renders a file card — icon, filename, fetched size — for a chat-attachment link', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(new Response(null, { status: 200, headers: { 'content-length': '2048' } })),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <MarkdownMessageStatic>{'[report.pdf](chats/c1/attachments/report.pdf)'}</MarkdownMessageStatic>
      </ChatMarkdownAssetProvider>,
    )

    expect(screen.getByText('report.pdf')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('2.00 KB')).toBeInTheDocument())
    vi.unstubAllGlobals()
  })

  it('falls back to the ordinary link with no MarkdownAssetContext at all', () => {
    render(<MarkdownMessageStatic>{'[report.pdf](chats/c1/attachments/report.pdf)'}</MarkdownMessageStatic>)
    expect(screen.getByRole('link', { name: 'report.pdf' })).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`
Expected: FAIL — every link still renders through the unmodified `LinkElement`, so the file-card assertions (icon/size) never appear.

- [ ] **Step 3: Write minimal implementation**

`chat-attachment-file-card.tsx`:

```tsx
'use client'

import { useEffect, useState } from 'react'
import type { TLinkElement } from 'platejs'
import { PlateElement, type PlateElementProps } from 'platejs/react'
import { LinkElement } from '@/components/ui/link-node'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { handleMarkdownAnchorClick } from '@/lib/markdown-link'
import { chatAttachmentUrl, fetchChatAttachmentMetadata, parseChatAttachmentRef } from './chat-asset-resolver'

/** Same units scheme as the file explorer's Properties dialog
 *  (use-file-explorer-context-menu.tsx), kept independent since that helper
 *  isn't exported. */
function formatAttachmentSize(bytes: number | null): string | null {
  if (bytes === null || !Number.isFinite(bytes) || bytes < 0) return null
  if (bytes < 1024) return `${bytes} bytes`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

/**
 * `[filename](ref)` for a real chat attachment renders as a file card
 * instead of a bare link. Falls through to the ordinary `LinkElement` for
 * everything else: a `ref` that isn't `chats/{chatId}/attachments/{file}`,
 * or no `MarkdownAssetContext` at all.
 */
export function ChatLinkElement(props: PlateElementProps<TLinkElement>) {
  const asset = useMarkdownAsset()
  const ref = props.element.url ?? ''
  const parsed = parseChatAttachmentRef(ref)

  if (!parsed || !asset) return <LinkElement {...props} />

  return <ChatAttachmentFileCard {...props} wsId={asset.wsId} attachmentRef={ref} />
}

function ChatAttachmentFileCard({
  wsId,
  attachmentRef,
  ...props
}: PlateElementProps<TLinkElement> & { wsId: string; attachmentRef: string }) {
  const [size, setSize] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    void fetchChatAttachmentMetadata(wsId, attachmentRef).then((meta) => {
      if (!cancelled) setSize(meta?.size ?? null)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, attachmentRef])

  const href = chatAttachmentUrl(wsId, attachmentRef)
  const sizeLabel = formatAttachmentSize(size)
  const filename = parseChatAttachmentRef(attachmentRef)?.filename ?? ''

  return (
    <PlateElement
      {...props}
      as="a"
      className="chat-attachment-file-card inline-flex items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1 align-middle text-sm no-underline"
      attributes={{
        ...props.attributes,
        href: href ?? undefined,
        onClick: (e) => handleMarkdownAnchorClick(e, href ?? undefined),
      }}
    >
      <FileExplorerIcon fileName={filename} size={14} className="shrink-0" />
      <span className="truncate">{props.children}</span>
      {sizeLabel && <span className="shrink-0 text-muted-foreground text-xs">{sizeLabel}</span>}
    </PlateElement>
  )
}
```

`chat-link-kit.tsx`:

```tsx
'use client'

import { LinkRules } from '@platejs/link'
import { LinkPlugin } from '@platejs/link/react'
import { LinkFloatingToolbar } from '@/components/ui/link-toolbar'
import { ChatLinkElement } from './chat-attachment-file-card'

const inputRules = [
  LinkRules.markdown(),
  LinkRules.autolink({ variant: 'paste' }),
  LinkRules.autolink({ variant: 'space' }),
  LinkRules.autolink({ variant: 'break' }),
]

/** Same `LinkRules`/toolbar as the shared `LinkKit`
 *  (components/editor/plugins/link-kit.tsx) — only the node renderer
 *  differs: `ChatLinkElement` renders a file card for a chat-attachment
 *  ref, falling through to the ordinary `LinkElement` for everything else. */
export const ChatLinkKit = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElement, afterEditable: () => <LinkFloatingToolbar /> } }),
]

export const ChatLinkKitStatic = [
  LinkPlugin.configure({ inputRules, render: { node: ChatLinkElement } }),
]
```

In `chat-composer-plugins.ts`: replace

```ts
import { LinkKit, LinkKitStatic } from '@/components/editor/plugins/link-kit'
```

with

```ts
import { ChatLinkKit, ChatLinkKitStatic } from '@/features/agent/composer/plate/attachments/chat-link-kit'
```

then `...LinkKit` → `...ChatLinkKit`, and `[LinkPlugin.key]: LinkKitStatic[0]` → `[LinkPlugin.key]: ChatLinkKitStatic[0]`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`
Expected: PASS. Also re-run `chat-composer-plugins-static.test.ts` to confirm the key-parity check still holds.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/chat-link-kit.tsx web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx web/src/features/agent/composer/plate/chat-composer-plugins.ts web/src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx
git commit -m "feat(chat): render generic file attachments as a file card"
```

---

### Task 19: CSV table-vs-file resolver (`resolve-csv.ts`)

**Files:**
- Modify: `web/package.json` (add `papaparse`, `@types/papaparse`)
- Create: `web/src/features/agent/composer/plate/attachments/resolve-csv.ts`
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/resolve-csv.test.ts`

**Interfaces:**
- Consumes: `papaparse` (new dependency — a mature, widely-used RFC4180 implementation with quoted-field/embedded-comma/newline support and a real error-reporting API, chosen over a hand-rolled split since a naive comma-split is exactly the "silently corrupted data" failure mode the spec calls out)
- Produces: `resolveCsv(bytes: Uint8Array): {kind:'table', rows: string[][]} | {kind:'file'}`, `rowsToMarkdownTable(rows: string[][]): string`, `CSV_MAX_ROWS = 200`, `CSV_MAX_COLUMNS = 20` — editor-UX's drop/attach handler calls `resolveCsv`, and on `'table'` calls `rowsToMarkdownTable` to get the text to insert (rendered by the already-registered `TablePlugin`/`TableRowPlugin`/`TableCellPlugin`); on `'file'`, inserts a generic `[filename](ref)` file attachment instead

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import {
  CSV_MAX_COLUMNS,
  CSV_MAX_ROWS,
  resolveCsv,
  rowsToMarkdownTable,
} from '@/features/agent/composer/plate/attachments/resolve-csv'

const encode = (s: string) => new TextEncoder().encode(s)

describe('resolveCsv', () => {
  it('parses a small, well-formed CSV into a table', () => {
    expect(resolveCsv(encode('name,age\nAda,36\nGrace,85\n'))).toEqual({
      kind: 'table',
      rows: [
        ['name', 'age'],
        ['Ada', '36'],
        ['Grace', '85'],
      ],
    })
  })

  it('handles RFC4180 quoted fields with embedded commas and quotes', () => {
    expect(resolveCsv(encode('name,note\n"Smith, John","said ""hi"""\n'))).toEqual({
      kind: 'table',
      rows: [
        ['name', 'note'],
        ['Smith, John', 'said "hi"'],
      ],
    })
  })

  it('falls back to a file when a cell contains a `|`, which would corrupt GFM table syntax', () => {
    expect(resolveCsv(encode('name,formula\nx,"a|b"\n'))).toEqual({ kind: 'file' })
  })

  it('falls back to a file when a quoted cell embeds a real newline', () => {
    expect(resolveCsv(encode('name,note\nx,"line one\nline two"\n'))).toEqual({ kind: 'file' })
  })

  it('falls back to a file over the row cutoff', () => {
    const rows = Array.from({ length: CSV_MAX_ROWS + 1 }, (_, i) => `r${i},1`).join('\n')
    expect(resolveCsv(encode(`h1,h2\n${rows}\n`))).toEqual({ kind: 'file' })
  })

  it('falls back to a file over the column cutoff', () => {
    const header = Array.from({ length: CSV_MAX_COLUMNS + 1 }, (_, i) => `c${i}`).join(',')
    expect(resolveCsv(encode(`${header}\n`))).toEqual({ kind: 'file' })
  })

  it('falls back to a file on malformed CSV a real parser flags as an error', () => {
    expect(resolveCsv(encode('name,note\n"unterminated,x\n'))).toEqual({ kind: 'file' })
  })
})

describe('rowsToMarkdownTable', () => {
  it('serializes rows as a GFM table with a header separator', () => {
    expect(
      rowsToMarkdownTable([
        ['name', 'age'],
        ['Ada', '36'],
      ]),
    ).toBe('| name | age |\n| --- | --- |\n| Ada | 36 |')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/resolve-csv.test.ts`
Expected: FAIL — module doesn't exist (and `papaparse` isn't installed yet).

- [ ] **Step 3: Write minimal implementation**

```bash
cd web && bun add papaparse && bun add -d @types/papaparse
```

```ts
import Papa from 'papaparse'

export const CSV_MAX_ROWS = 200
export const CSV_MAX_COLUMNS = 20

export type CsvResolution = { kind: 'table'; rows: string[][] } | { kind: 'file' }

/** Two independent gates, both must pass for inline-as-table (design spec's
 *  "CSV table-vs-file" rule):
 *  1. Size — at most CSV_MAX_ROWS rows / CSV_MAX_COLUMNS columns. 200x20 is a
 *     ceiling for a table meant to be READ in a chat bubble, not a data
 *     browser; bigger belongs in a file card instead.
 *  2. Shape — parses cleanly with `papaparse` (RFC4180: quoted fields,
 *     embedded commas/quotes) with zero parse errors, and no cell contains a
 *     newline or a `|` that would corrupt GFM table syntax.
 *  Failing either gate returns `{ kind: 'file' }` — never a best-effort
 *  table, since inline CSV is delivered as ground truth the agent reads. */
export function resolveCsv(bytes: Uint8Array): CsvResolution {
  const text = new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  const result = Papa.parse<string[]>(text, { skipEmptyLines: true })
  if (result.errors.length > 0) return { kind: 'file' }

  const rows = result.data
  if (rows.length === 0) return { kind: 'file' }
  const columns = Math.max(...rows.map((row) => row.length))
  if (rows.length > CSV_MAX_ROWS || columns > CSV_MAX_COLUMNS) return { kind: 'file' }
  if (rows.some((row) => row.some((cell) => cell.includes('\n') || cell.includes('|')))) {
    return { kind: 'file' }
  }

  return { kind: 'table', rows }
}

/** Serializes resolved rows as a GFM markdown table (first row = header).
 *  Cells are NOT re-escaped for `|` — `resolveCsv` already rejects any cell
 *  containing one, so a `'table'` resolution is always safe verbatim. */
export function rowsToMarkdownTable(rows: string[][]): string {
  if (rows.length === 0) return ''
  const [header, ...body] = rows
  const columnCount = header.length
  const pad = (row: string[]) => Array.from({ length: columnCount }, (_, i) => (row[i] ?? '').trim())
  const line = (row: string[]) => `| ${pad(row).join(' | ')} |`
  const separator = `| ${Array.from({ length: columnCount }, () => '---').join(' | ')} |`
  return [line(header), separator, ...body.map(line)].join('\n')
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/resolve-csv.test.ts`
Expected: PASS. If the "malformed CSV" case doesn't actually populate `result.errors` for this papaparse version, swap in a fixture papaparse's changelog documents as an error case (e.g. `TooFewFields`/`TooManyFields` via `{ delimiter: ',' , transform }`) — the gate's correctness, not this exact fixture, is what matters.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/bun.lock web/src/features/agent/composer/plate/attachments/resolve-csv.ts web/src/__tests__/features/agent/composer/plate/attachments/resolve-csv.test.ts
git commit -m "feat(chat): add the CSV table-vs-file resolver"
```

---

**Frontend rendering API summary (for Phase 3's editor-UX tasks):**

- Node/plugin registration is fully done here — nothing left for editor-UX to register in `chat-composer-plugins.ts`.
- **Encoding contracts editor-UX must produce when inserting nodes:**
  - Text attachment: a `code_block` node, `lang: 'text-attachment:' + id`, children = code lines holding the raw pasted text verbatim
  - Excalidraw: a `code_block` node, `lang: 'excalidraw:' + id`, children = the scene JSON; **immediately followed in the same parent** by an `img` node (`type:'img', url: <png ref>`)
  - Image: `img` node, `url: ref` (unchanged, `MarkdownImageKit`'s existing shape)
  - File: `a`/link node, `url: ref`, text children = filename (unchanged, ordinary link shape)
  - **Id shape** (hard requirement): `/^[A-Za-z0-9_-]{6,}$/` — a default `nanoid()` (already used elsewhere — `pane-layout.ts`, `buffer-slice.ts`) of length ≥6 satisfies this.
  - **Ref shape** (hard requirement): `chats/{chatId}/attachments/{filename}` exactly — must match what the upload endpoint returns verbatim.
- **CSV**: call `resolveCsv(bytes)` from `resolve-csv.ts` on drop/attach, and on `'table'` insert `rowsToMarkdownTable(rows)` as plain markdown text at the cursor; on `'file'`, insert a normal `[filename](ref)` file attachment.
- **Asset resolution**, if the embedded Excalidraw editor needs to preview an already-uploaded PNG or check a ref: `chatMarkdownAssetInfo(wsId)`, `parseChatAttachmentRef(ref)`, `chatAttachmentUrl(wsId, ref)`, `fetchChatAttachmentDataUrl(wsId, ref)`, `fetchChatAttachmentMetadata(wsId, ref)` — all from `chat-asset-resolver.ts`.
- **URL convention**: attachment URLs are built via `chatBase(wsId)` (now exported from `agent-api.ts`), which resolves through the app's real hierarchical route (`/v0/projects/{p}/repos/{r}/workspaces/{w}/chats/{chatId}/attachments/{filename}`) — matching backend Task 4's route registration.

# Phase 3: Frontend — Editor UX

**File Structure:**

Create (new, not covered by Phase 2):
- `web/src/features/agent/api/upload-chat-attachment.ts` — the client upload function every other task in this phase calls
- `web/src/features/agent/composer/lib/attachment-markdown.ts` — pure builders for the four fenced/link markdown encodings
- `web/src/features/agent/composer/lib/paste-threshold.ts` — pure paste-length-threshold decision + shift-state tracking helper
- `web/src/features/agent/composer/composer-plus-button.tsx` — the plus button + its two-entry dropdown
- `web/src/features/agent/composer/attach-file-modal.tsx` — drag-and-drop + click-to-browse upload modal
- `web/src/features/agent/composer/excalidraw-modal.tsx` — lightweight modal chrome, lazy-loads the heavy canvas
- `web/src/features/agent/composer/excalidraw-canvas.tsx` — the actual `<Excalidraw>` mount + save/export logic (the lazy chunk)
- `web/src/features/agent/composer/plate/chat-paste-plugin.ts` — the paste-interception Plate plugin
- `web/src/features/agent/composer/plate/attachment-drag-handle.tsx` — reusable `useAttachmentDraggable`/`AttachmentDragHandle`/`AttachmentDropLine` primitive (wired into Phase 2's node components in Phase 4)
- `web/src/features/file-system/lib/tauri-file-drop.ts` — `useTauriFileDrop` hook + `pointInRect` pure helper (the real Tauri `onDragDropEvent` path)

Modify:
- `web/src/features/agent/composer/lib/handle-geometry.ts` — two-slot button-cluster geometry
- `web/src/features/agent/composer/composer-handle.tsx` — renders the plus button alongside send
- `web/src/features/agent/composer/composer-field.tsx` — forwards a ref down to `ChatMarkdownEditor`
- `web/src/features/agent/composer/plate/chat-markdown-editor.tsx` — imperative insert handle, registers the paste plugin, accepts `wsId`/`chatId`
- `web/src/features/agent/composer/agent-composer.tsx` — owns modal-open state, wires the plus button, wires composer-level drop
- `web/src/features/agent/chat/agent-empty-document.tsx` — plus-button + paste parity on the blank-chat surface
- `web/src/features/agent/chat/agent-chat-view.tsx` — passes `wsId`/`chatId` into `AgentEmptyDocument`
- `web/src/features/agent/shared/agent-icons.tsx` — adds `PlusIcon`, `FileIcon`, `PencilIcon`
- `web/src/features/agent/styles/composer.css` — `.plusbtn`, `.handle .send` margin override, drop-target state, modal/dropzone styles
- `web/src/features/file-system/utils/file-system-dropped-paths.ts` — honest doc update (still `[]`; real paths now come from `useTauriFileDrop`, not this function)
- `web/src/features/panes/components/pane-container.tsx` — wires `useTauriFileDrop` alongside the existing dead DOM path
- `web/src/features/terminal/components/terminal.tsx` — same
- `web/package.json` — adds `@excalidraw/excalidraw`

**Key architectural findings (read this before the tasks):**

1. **The plus button's home is already flex, already has a spacing rule for a second occupant.** `.agent-chat .handle` (`composer-handle.tsx` / `composer.css:426`) is `display:flex; gap:2px`, and `.agent-chat .send` (`composer.css:446`) carries `margin-left:5px` that is currently *inert* — dead weight, since `.send` is `.handle`'s only child today. `.agent-chat .pill .field`'s `padding-right:62px` (`composer.css:327`) already reserves exactly `sendInset() [4] + 2×SEND_DIAMETER [56] + one 2px gap = 62` — precisely a second 28px circle's worth of room, provided the two buttons' gap stays at 2px (not 2+5=7). That means the field's own padding needs **zero changes**; the only CSS debt is a one-line override (`.handle .send { margin-left: 0 }`) to stop double-counting the now-meaningful margin against the flex gap.
2. **The plus button sits to the right of send** (spec, literally) — meaning it becomes the new flush-right element and send moves one slot inward. DOM order inside `.handle` is `[send, plus]`.
3. **`BlockMenuKit` does not contain a drag-to-reorder handle in this codebase.** `block-selection-kit.tsx:6` references a `dnd-kit.tsx` that was never actually added — `BlockMenuKit` = `BlockSelectionKit` (click/shift-click block highlighting) + `BlockMenuPlugin` (right-click context menu) only. The only real drag-reorder primitive in the repo is `@platejs/dnd`'s `useDraggable`/`useDropLine`, used today for table-row reordering (`components/ui/table-node.tsx:1087-1187`). So the spec's open question resolves itself: there is no `BlockMenuKit` subset to trim — a standalone handle built on `@platejs/dnd` directly is the only viable path.
4. **`extractDroppedFilePaths(dataTransfer)` can never be revived as written.** `desktop/src-tauri/tauri.conf.json` sets no `dragDropEnabled` key anywhere, so Tauri v2's default (`true`, native OS drag-drop interception) is in effect. Under that default, Tauri's webview intercepts a real OS file drag *before* it becomes a DOM `drop` event at all — the existing `handleDrop`/`handleTerminalFileDrop` call sites are dead for real file drops on the desktop build regardless of what `extractDroppedFilePaths` does with the `DataTransfer` it's handed. Real paths only arrive via `@tauri-apps/api/webview`'s `getCurrentWebview().onDragDropEvent()`, which is a window-wide event carrying `{paths, position}` — not scoped to an element. The fix is a new `useTauriFileDrop` hook that every consumer (pane, terminal, composer, attach-file modal) mounts against its own container ref, filtering the shared event by `position` falling inside its own `getBoundingClientRect()`. `extractDroppedFilePaths` itself stays `[]` — that's now honestly correct (a `DataTransfer` genuinely never carries a host path in a browser), not a stub.
5. **Paste/keydown interception must be a Plate plugin's `handlers`, not the `PlateContent` DOM prop.** `chat-markdown-editor.tsx:81-89`'s own comment establishes why for Enter; the same ordering applies to paste — Slate's default paste-insertion runs before a DOM `onPaste` prop would ever get a chance to `preventDefault()`.
6. **`chatMarkdownToValue`/`createMarkdownCodec`** (`chat-composer-serialization.ts`, `markdown-codec.ts`) already turn a markdown string into a `Value` bound to `chatComposerPlugins`. Every attachment-insertion task builds a markdown *string* (fenced block or `![]()`/`[]()` link) and deserializes it through this existing codec rather than constructing raw Slate nodes by hand — this is what lets the editor-UX tasks stay decoupled from Phase 2's exact node shapes.

---

### Task 20: `uploadChatAttachment` client function

**Files:**
- Create: `web/src/features/agent/api/upload-chat-attachment.ts`
- Test: `web/src/__tests__/features/agent/api/upload-chat-attachment.test.ts`

**Interfaces:**
- Consumes: `chatBase` (`@/features/agent/api/agent-api`, Task 10), `API_BASE` (`@/lib/api`), `nanoid` (`nanoid` package)
- Produces: `UploadedChatAttachment { ref, filename, size, contentType }`, `UploadChatAttachmentInput = {file: File} | {path: string}`, `uploadChatAttachment(wsId, chatId, input, id?: string): Promise<UploadedChatAttachment>` — every remaining task in this phase calls this

This is the one piece of glue nobody upstream builds: Phase 1 builds the endpoint, Phase 2 builds the resolvers that read an already-stored ref back, but nothing yet turns a `File`/host path into an actual `fetch()` call. `id` is optional and minted here by default via `nanoid()` — most callers (drop, Attach File, paste-to-image) don't need to correlate it with anything else. The Excalidraw task (Task 34) is the one exception: it needs a SINGLE id to serve both the fence-tag id (the JSON) and the filename shortid (the PNG), so it generates its own and passes it through explicitly rather than letting this function mint one.

- [ ] **Step 1: Write the failing test**

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

beforeEach(() => {
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

function jsonResponse(body: unknown, status = 201) {
  return new Response(JSON.stringify(body), { status })
}

describe('uploadChatAttachment', () => {
  it('uploads a File as multipart and maps the response envelope', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        data: { ref: 'chats/c1/attachments/x-photo.png', fileName: 'x-photo.png', size: 5, contentType: 'image/png' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const file = new File(['hello'], 'photo.png', { type: 'image/png' })
    const result = await uploadChatAttachment('ws1', 'c1', { file }, 'x')

    expect(result).toEqual({
      ref: 'chats/c1/attachments/x-photo.png',
      filename: 'x-photo.png',
      size: 5,
      contentType: 'image/png',
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toContain('/chats/c1/attachments')
    expect(init.body).toBeInstanceOf(FormData)
    vi.unstubAllGlobals()
  })

  it('uploads a host path as a JSON body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        data: { ref: 'chats/c1/attachments/x-dropped.png', fileName: 'x-dropped.png', size: 9, contentType: 'image/png' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const result = await uploadChatAttachment('ws1', 'c1', { path: '/tmp/dropped.png' }, 'x')

    expect(result.ref).toBe('chats/c1/attachments/x-dropped.png')
    const [, init] = fetchMock.mock.calls[0]
    expect(JSON.parse(init.body as string)).toEqual({ path: '/tmp/dropped.png', id: 'x' })
    vi.unstubAllGlobals()
  })

  it('mints its own id, matching the shape the fence-tag parser requires, when none is passed', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: { ref: 'r', fileName: 'f', size: 1, contentType: 'image/png' } }))
    vi.stubGlobal('fetch', fetchMock)

    await uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' })

    const [, init] = fetchMock.mock.calls[0]
    const body = JSON.parse(init.body as string)
    expect(body.id).toMatch(/^[A-Za-z0-9_-]{6,}$/)
    vi.unstubAllGlobals()
  })

  it('throws on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('too big', { status: 413 })))
    await expect(uploadChatAttachment('ws1', 'c1', { path: '/tmp/x.png' }, 'x')).rejects.toThrow()
    vi.unstubAllGlobals()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/api/upload-chat-attachment.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

```ts
import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-api'
import { API_BASE } from '@/lib/api'

export interface UploadedChatAttachment {
  ref: string
  filename: string
  size: number
  contentType: string
}

export type UploadChatAttachmentInput = { file: File } | { path: string }

interface UploadAttachmentResponseBody {
  data: { ref: string; fileName: string; size: number; contentType: string }
}

/**
 * Uploads one chat attachment and returns the durable reference to encode
 * into the message's markdown (`![alt](ref)` / `[name](ref)`).
 *
 * Two request shapes, matching the backend's two ingestion paths: multipart
 * bytes for a `File` (clipboard paste, browser picker — never has a host
 * path), or a JSON `{path, id}` body for a host path from a revived desktop
 * drop (the daemon reads it itself).
 */
export async function uploadChatAttachment(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
  id: string = nanoid(),
): Promise<UploadedChatAttachment> {
  const url = `${API_BASE}${chatBase(wsId)}/${encodeURIComponent(chatId)}/attachments`
  const response =
    'file' in input
      ? await fetch(url, { method: 'POST', body: attachmentFormData(input.file, id) })
      : await fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: input.path, id }),
        })

  if (!response.ok) {
    throw new Error(`upload chat attachment failed: ${response.status} ${await response.text()}`)
  }
  const { data } = (await response.json()) as UploadAttachmentResponseBody
  return { ref: data.ref, filename: data.fileName, size: data.size, contentType: data.contentType }
}

function attachmentFormData(file: File, id: string): FormData {
  const body = new FormData()
  body.append('id', id)
  body.append('file', file, file.name)
  return body
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/api/upload-chat-attachment.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/api/upload-chat-attachment.ts web/src/__tests__/features/agent/api/upload-chat-attachment.test.ts
git commit -m "feat(chat): add the client-side chat attachment upload function"
```

---

### Task 21: Composer Handle Geometry — Two-Slot Button Cluster

**Files:**
- Modify: `web/src/features/agent/composer/lib/handle-geometry.ts`
- Test: `web/src/__tests__/features/agent/composer/lib/handle-geometry.test.ts` (existing file — extend)

**Interfaces:**
- Consumes: nothing new (extends `SEND_DIAMETER`, `sendInset()` already in this file)
- Produces: `PLUS_DIAMETER`, `HANDLE_GAP`, `handleClusterWidth(occupants?, diameter?, gap?): number`, `fieldRightPadding(rightInset?, occupants?, diameter?, gap?): number`

- [ ] **Step 1: Write the failing test**

Append to `web/src/__tests__/features/agent/composer/lib/handle-geometry.test.ts`:

```ts
import {
  COMPOSER_LINE_HEIGHT,
  handleOffset,
  isMultiline,
  sendInset,
  SEND_DIAMETER,
  handleClusterWidth,
  fieldRightPadding,
  HANDLE_GAP,
  PLUS_DIAMETER,
} from '@/features/agent/composer/lib/handle-geometry'

describe('handleClusterWidth', () => {
  it('is one circle wide with a single occupant', () => {
    expect(handleClusterWidth(1)).toBe(SEND_DIAMETER)
  })

  it('adds one diameter and one gap for the second occupant', () => {
    expect(handleClusterWidth(2)).toBe(58)
  })

  it('the plus button matches the send button diameter', () => {
    expect(PLUS_DIAMETER).toBe(SEND_DIAMETER)
  })
})

describe('fieldRightPadding', () => {
  // THE TWO ARE ONE NUMBER, same as sendInset: this must equal the literal
  // `padding-right` composer.css already ships on `.pill .field`.
  it('matches the shipped field padding-right exactly, for two occupants', () => {
    expect(fieldRightPadding()).toBe(62)
  })

  it('shrinks to the single-occupant reservation', () => {
    expect(fieldRightPadding(sendInset(), 1)).toBe(32)
  })

  it('follows HANDLE_GAP', () => {
    expect(fieldRightPadding(4, 2, 28, 3)).toBe(4 + 28 + 3 + 28)
  })
})
```

(Update the existing top-of-file import block to add the new named imports rather than duplicating the `import` statement.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/handle-geometry.test.ts`
Expected: FAIL — `handleClusterWidth`, `fieldRightPadding`, `HANDLE_GAP`, `PLUS_DIAMETER` are not exported yet.

- [ ] **Step 3: Write minimal implementation**

Append to `web/src/features/agent/composer/lib/handle-geometry.ts`:

```ts
/** The plus button's diameter — the SAME circle as send, so the two read as
 *  one control family rather than two mismatched controls glued together. */
export const PLUS_DIAMETER = SEND_DIAMETER

/** `.handle`'s own flex `gap`, between however many buttons it holds. */
export const HANDLE_GAP = 2

/**
 * The button cluster's total width, edge to edge, for however many circles
 * `.handle` holds (2 today: plus + send).
 *
 * THIS NUMBER AND THE FIELD'S OWN `padding-right` ARE ONE NUMBER, same rule
 * as `sendInset`: the field reserves exactly `sendInset() + this` so wrapped
 * text never renders under a button. Add a third control without widening
 * the field to match and text starts running underneath it.
 */
export function handleClusterWidth(
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return occupants * diameter + Math.max(0, occupants - 1) * gap
}

/**
 * The field's own right padding, derived rather than picked.
 *
 * At the shipped values (4px inset, two 28px circles, one 2px gap) that is
 * 4 + 28 + 2 + 28 = 62 — exactly what `.agent-chat .pill .field` already
 * ships as `padding-right`, with zero slack to spare. That is not
 * coincidence papered over after the fact: the field was already sized for
 * a second control before this one existed. It is also why adding the plus
 * button requires zeroing `.handle .send`'s own `margin-left` in
 * composer.css — keeping BOTH that margin and `.handle`'s flex `gap` would
 * double-count the space between the two buttons and blow through this
 * budget by exactly `SEND_MARGIN_LEFT`'s old value (5px).
 */
export function fieldRightPadding(
  rightInset: number = sendInset(),
  occupants: number = 2,
  diameter: number = SEND_DIAMETER,
  gap: number = HANDLE_GAP,
): number {
  return rightInset + handleClusterWidth(occupants, diameter, gap)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/handle-geometry.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/lib/handle-geometry.ts web/src/__tests__/features/agent/composer/lib/handle-geometry.test.ts
git commit -m "feat(composer): derive two-slot button cluster geometry"
```

---

### Task 22: `.plusbtn` Icons and CSS

**Files:**
- Modify: `web/src/features/agent/shared/agent-icons.tsx`
- Modify: `web/src/features/agent/styles/composer.css`
- Test: `web/src/__tests__/features/agent/shared/agent-icons.test.tsx`

**Interfaces:**
- Produces: `PlusIcon`, `FileIcon`, `PencilIcon` (React components, same `AgentIconProps` shape as `StopIcon`/`UpIcon`); `.plusbtn` / `.handle .send{margin-left:0}` CSS classes consumed by Task 23. `PencilIcon` is needed by Task 24's Excalidraw dropdown entry.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/shared/agent-icons.test.tsx`:

```tsx
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { FileIcon, PencilIcon, PlusIcon } from '@/features/agent/shared/agent-icons'

afterEach(cleanup)

describe('PlusIcon / FileIcon / PencilIcon', () => {
  it('render on the shared 24-unit grid at the requested size', () => {
    const { container: plus } = render(<PlusIcon size={16} />)
    const plusSvg = plus.querySelector('svg')
    expect(plusSvg).toHaveAttribute('viewBox', '0 0 24 24')
    expect(plusSvg).toHaveAttribute('width', '16')

    const { container: file } = render(<FileIcon />)
    expect(file.querySelector('svg')).toHaveAttribute('width', '14')

    const { container: pencil } = render(<PencilIcon size={14} />)
    expect(pencil.querySelector('svg')).toHaveAttribute('width', '14')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/shared/agent-icons.test.tsx`
Expected: FAIL — `PlusIcon`/`FileIcon`/`PencilIcon` not exported.

- [ ] **Step 3: Write minimal implementation**

Append to `web/src/features/agent/shared/agent-icons.tsx`:

```tsx
export const PlusIcon = icon(<path d="M12 5.5v13M5.5 12h13" />, 'PlusIcon')

export const FileIcon = icon(
  <>
    <path d="M7 3.5h7l4.5 4.5v12.5H7z" />
    <path d="M14 3.5v4.5h4.5" />
  </>,
  'FileIcon',
)

/** The Excalidraw dropdown entry's icon — a plain pencil, distinguishing
 *  "open a drawing editor" from FileIcon's "pick an existing file". */
export const PencilIcon = icon(
  <path d="M4 20l1-4.5L15.5 5 19 8.5 8.5 19 4 20z" />,
  'PencilIcon',
)
```

Append to `web/src/features/agent/styles/composer.css` (right after the existing `.agent-chat .send.halt` block, `composer.css:494`):

```css
/* The plus button's own left margin is unset — see handle-geometry.ts's
   fieldRightPadding() derivation. `.handle`'s flex `gap: 2px` is the ONLY
   spacing between it and `.send`; keeping `.send`'s base margin-left: 5px
   here too would double-count against the field's exactly-sized padding. */
.agent-chat .handle .send {
  margin-left: 0;
}

/* Neutral, deliberately not `.send`'s primary treatment — "not the action".
   Same 28px circle so the pair reads as one control family. */
.agent-chat .plusbtn {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1px solid var(--input);
  background: var(--muted);
  color: var(--muted-foreground);
  cursor: pointer;
  transition:
    background-color 0.12s ease,
    border-color 0.12s ease,
    color 0.12s ease;
}
.agent-chat .plusbtn:hover,
.agent-chat .plusbtn[data-open] {
  background: color-mix(in oklch, var(--foreground) 8%, var(--muted));
  color: var(--foreground);
  border-color: color-mix(in oklch, var(--foreground) 22%, var(--input));
}
.agent-chat .plusbtn:active {
  transform: scale(0.94);
}
.agent-chat .plusbtn svg:not([class*='size-']) {
  width: 16px;
  height: 16px;
  stroke-width: 1.9;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/shared/agent-icons.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/shared/agent-icons.tsx web/src/features/agent/styles/composer.css web/src/__tests__/features/agent/shared/agent-icons.test.tsx
git commit -m "feat(composer): add plus/file/pencil icons and neutral plus-button styling"
```

---

### Task 23: `ComposerPlusButton` (button + two-entry dropdown)

**Files:**
- Create: `web/src/features/agent/composer/composer-plus-button.tsx`
- Test: `web/src/__tests__/features/agent/composer/composer-plus-button.test.tsx`

**Interfaces:**
- Consumes: `DropdownMenu`/`DropdownMenuTrigger`/`DropdownMenuContent`/`DropdownMenuItem` (`@/components/ui/dropdown-menu`), `PlusIcon`/`PencilIcon`/`FileIcon` (Task 22, `agent-icons.tsx`)
- Produces: `ComposerPlusButton({ onOpenExcalidraw: () => void; onOpenAttachFile: () => void })` — consumed by Task 24

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/composer-plus-button.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'

afterEach(cleanup)

describe('ComposerPlusButton', () => {
  it('opens a dropdown with exactly Excalidraw and Attach File', async () => {
    render(<ComposerPlusButton onOpenExcalidraw={vi.fn()} onOpenAttachFile={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /add to this message/i }))
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((el) => el.textContent)).toEqual(['Excalidraw', 'Attach File'])
  })

  it('calls onOpenExcalidraw for the Excalidraw entry, not onOpenAttachFile', async () => {
    const onOpenExcalidraw = vi.fn()
    const onOpenAttachFile = vi.fn()
    render(<ComposerPlusButton onOpenExcalidraw={onOpenExcalidraw} onOpenAttachFile={onOpenAttachFile} />)
    fireEvent.click(screen.getByRole('button', { name: /add to this message/i }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    expect(onOpenExcalidraw).toHaveBeenCalledTimes(1)
    expect(onOpenAttachFile).not.toHaveBeenCalled()
  })

  it('calls onOpenAttachFile for the Attach File entry', async () => {
    const onOpenExcalidraw = vi.fn()
    const onOpenAttachFile = vi.fn()
    render(<ComposerPlusButton onOpenExcalidraw={onOpenExcalidraw} onOpenAttachFile={onOpenAttachFile} />)
    fireEvent.click(screen.getByRole('button', { name: /add to this message/i }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /attach file/i }))
    expect(onOpenAttachFile).toHaveBeenCalledTimes(1)
    expect(onOpenExcalidraw).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/composer-plus-button.test.tsx`
Expected: FAIL — module `composer-plus-button.tsx` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/composer-plus-button.tsx`:

```tsx
import { useState } from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { FileIcon, PencilIcon, PlusIcon } from '@/features/agent/shared/agent-icons'

interface ComposerPlusButtonProps {
  onOpenExcalidraw: () => void
  onOpenAttachFile: () => void
}

/**
 * The composer's second control, styled neutrally — it opens a choice, it
 * does not send anything, so it never borrows send's primary treatment.
 *
 * Two entries only, per the design spec: Excalidraw and Attach File are
 * deliberately NOT the same entry point, because Excalidraw opens an editor
 * (produces a scene) while Attach File opens a picker (ingests an existing
 * file) — kind for a picked file is inferred from the file itself, kind for
 * this entry is fixed by which one was clicked.
 */
export function ComposerPlusButton({ onOpenExcalidraw, onOpenAttachFile }: ComposerPlusButtonProps) {
  const [open, setOpen] = useState(false)

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger
        className="plusbtn"
        data-open={open || undefined}
        aria-label="Add to this message"
        title="Add to this message"
      >
        <PlusIcon size={16} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" sideOffset={8}>
        <DropdownMenuItem onClick={onOpenExcalidraw}>
          <PencilIcon size={14} />
          <span>Excalidraw</span>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onOpenAttachFile}>
          <FileIcon size={14} />
          <span>Attach File</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/composer-plus-button.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/composer-plus-button.tsx web/src/__tests__/features/agent/composer/composer-plus-button.test.tsx
git commit -m "feat(composer): add plus button with Excalidraw/Attach File dropdown"
```

---

### Task 24: Wire the Plus Button into `ComposerHandle` / `AgentComposer`

**Files:**
- Modify: `web/src/features/agent/composer/composer-handle.tsx`
- Modify: `web/src/features/agent/composer/agent-composer.tsx`
- Test: `web/src/__tests__/features/agent/composer/composer-handle.test.tsx`

**Interfaces:**
- Consumes: `ComposerPlusButton` (Task 23)
- Produces: `ComposerHandle` gains `onOpenExcalidraw`/`onOpenAttachFile` props; `AgentComposer` owns `const [modal, setModal] = useState<'excalidraw' | 'attach-file' | null>(null)` — consumed by Task 29 (Attach File modal) and Task 34 (Excalidraw modal), which each replace the `null` branches of `modal === '...'` with their real modal component

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/composer-handle.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposerHandle } from '@/features/agent/composer/composer-handle'

afterEach(cleanup)

const baseProps = {
  fieldHeight: 20,
  hasText: false,
  working: false,
  canStop: false,
  sending: false,
  onSend: vi.fn(),
  onStop: vi.fn(),
  onOpenExcalidraw: vi.fn(),
  onOpenAttachFile: vi.fn(),
}

describe('ComposerHandle', () => {
  it('renders send and plus as siblings, send first in DOM order', () => {
    render(<ComposerHandle {...baseProps} />)
    const send = screen.getByRole('button', { name: /send — enter/i })
    const plus = screen.getByRole('button', { name: /add to this message/i })
    expect(send.compareDocumentPosition(plus) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('opens Excalidraw from the plus dropdown without touching send', async () => {
    render(<ComposerHandle {...baseProps} />)
    fireEvent.click(screen.getByRole('button', { name: /add to this message/i }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /excalidraw/i }))
    expect(baseProps.onOpenExcalidraw).toHaveBeenCalledTimes(1)
    expect(baseProps.onSend).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/composer-handle.test.tsx`
Expected: FAIL — `ComposerHandle` does not accept/render `onOpenExcalidraw`/plus button yet.

- [ ] **Step 3: Write minimal implementation**

Edit `web/src/features/agent/composer/composer-handle.tsx`:

```tsx
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'
import { handleOffset } from '@/features/agent/composer/lib/handle-geometry'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'

interface ComposerHandleProps {
  fieldHeight: number
  hasText: boolean
  working: boolean
  canStop: boolean
  sending: boolean
  onSend: () => void
  onStop: () => void
  /** Opens the embedded Excalidraw editor. */
  onOpenExcalidraw: () => void
  /** Opens the drag-and-drop/click-to-browse file picker. */
  onOpenAttachFile: () => void
}

export function ComposerHandle({
  fieldHeight,
  hasText,
  working,
  canStop,
  sending,
  onSend,
  onStop,
  onOpenExcalidraw,
  onOpenAttachFile,
}: ComposerHandleProps) {
  const stopping = working && canStop
  const sendingVisual = !stopping && !hasText && sending
  const idle = !stopping && !sendingVisual && !hasText

  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      <button
        type="button"
        className={cn('send', stopping && 'halt', (idle || sendingVisual) && 'off')}
        disabled={idle || sendingVisual}
        aria-label={stopping ? 'Stop this turn' : sendingVisual ? 'Sending' : 'Send prompt'}
        title={stopping ? 'Stop this turn — Esc' : sendingVisual ? 'Sending…' : 'Send — Enter'}
        onClick={stopping ? onStop : onSend}
      >
        {stopping ? (
          <StopIcon size={16} />
        ) : sendingVisual ? (
          <FlickerSpinner className="size-4" />
        ) : (
          <UpIcon size={16} />
        )}
      </button>
      <ComposerPlusButton onOpenExcalidraw={onOpenExcalidraw} onOpenAttachFile={onOpenAttachFile} />
    </div>
  )
}
```

Edit `web/src/features/agent/composer/agent-composer.tsx` — add local modal state and thread the two new callbacks (in the `'input'`/`'compacting'` render branch, `agent-composer.tsx:71-144`):

```tsx
// near the top of the component, alongside `const state = resolveComposerState(...)`
const [modal, setModal] = useState<'excalidraw' | 'attach-file' | null>(null)
```

```tsx
          <ComposerHandle
            fieldHeight={props.fieldHeight}
            hasText={props.draft.trim().length > 0}
            working={props.working}
            canStop={props.canStop}
            sending={props.sending}
            onSend={props.onSend}
            onStop={props.onStop}
            onOpenExcalidraw={() => setModal('excalidraw')}
            onOpenAttachFile={() => setModal('attach-file')}
          />
        </div>
        {/* Task 29 (AttachFileModal) and Task 34 (ExcalidrawModal) each replace
            their `null` branch below with the real modal, wired to close via
            `setModal(null)` and insert via the imperative handle from Task 25. */}
        {modal === 'attach-file' && null}
        {modal === 'excalidraw' && null}
```

(add `import { useState } from 'react'` to the top of `agent-composer.tsx`)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/composer-handle.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/composer-handle.tsx web/src/features/agent/composer/agent-composer.tsx web/src/__tests__/features/agent/composer/composer-handle.test.tsx
git commit -m "feat(composer): wire plus button into the send handle and composer"
```

---

### Task 25: Imperative Attachment-Insert Handle on `ChatMarkdownEditor`

**Files:**
- Modify: `web/src/features/agent/composer/plate/chat-markdown-editor.tsx`
- Modify: `web/src/features/agent/composer/composer-field.tsx`
- Modify: `web/src/features/agent/composer/agent-composer.tsx`
- Test: `web/src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx`

**Interfaces:**
- Consumes: `chatMarkdownToValue` (`chat-composer-serialization.ts`, already imported)
- Produces: `ChatMarkdownEditorHandle { insertAttachmentMarkdown(markdown: string): void }`, forwarded through `ComposerField`'s new `ref` prop — consumed by Task 29 (Attach File modal), Task 32 (composer drop wiring), Task 34 (Excalidraw modal)

This is the general escape hatch every later "insert an attachment from *outside* the editable" task needs, mirroring the existing `AgentEmptyDocumentHandle`/`getHandleRect` imperative-handle idiom already in this codebase (`agent-empty-document.tsx:30-37`).

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx`:

```tsx
import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRef } from 'react'
import {
  ChatMarkdownEditor,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'

afterEach(cleanup)

describe('ChatMarkdownEditor imperative handle', () => {
  it('inserts a block-level node built from markdown, and reports it via onChange', () => {
    const ref = createRef<ChatMarkdownEditorHandle>()
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        ref={ref}
        wsId="w1"
        chatId="c1"
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )

    ref.current?.insertAttachmentMarkdown('```text-attachment:abc123\nhello world\n```')

    const lastCall = onChange.mock.calls.at(-1)?.[0] as string
    expect(lastCall).toContain('text-attachment:abc123')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx`
Expected: FAIL — `ChatMarkdownEditor` accepts no `ref`, exports no `ChatMarkdownEditorHandle`, and doesn't yet accept `wsId`/`chatId` (added properly in Task 32 alongside paste interception — for this task, accept and ignore them, or stub with `wsId=''`/`chatId=''` defaults so the type-check passes without the paste plugin existing yet).

- [ ] **Step 3: Write minimal implementation**

Edit `web/src/features/agent/composer/plate/chat-markdown-editor.tsx`:

```tsx
import { useCallback, useImperativeHandle, useLayoutEffect, useMemo, useRef } from 'react'
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import { PointApi, RangeApi, type Value } from 'platejs'
import { createPlatePlugin, Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { cn } from '@/lib/utils'

export interface CaretEdges {
  atStart: boolean
  atEnd: boolean
}

/** What a caller OUTSIDE the editable can do to it — the same imperative-
 *  handle shape `AgentEmptyDocumentHandle` already uses for `getHandleRect`,
 *  for the same reason: the plus button, the Attach File modal, and the
 *  Excalidraw modal all live outside `<Plate>`'s own tree, so `useEditorRef`
 *  is not reachable from them. */
export interface ChatMarkdownEditorHandle {
  /**
   * Inserts a block-level void node at the current selection (end of
   * document if there is none), built by deserializing `markdown` through
   * this editor's own `chatComposerPlugins`-bound codec — the same path a
   * paste of that text would take, so a fenced `text-attachment`/`excalidraw`
   * block or an `![alt](ref)` image lands as the exact node shape the
   * rendering-side plugins expect.
   */
  insertAttachmentMarkdown(markdown: string): void
}

export interface ChatMarkdownEditorProps {
  /** Threaded through starting Task 32, when the paste-interception plugin
   *  needs it to call `uploadChatAttachment`. Unused before that — accepted
   *  now so every call site added in this phase compiles against one stable
   *  prop shape. */
  wsId: string
  chatId: string
  initialValue: string
  placeholder: string
  ariaLabel: string
  onChange: (markdown: string) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => void
  onHeightChange?: (height: number) => void
  autoFocus?: boolean
  expanded?: boolean
  controls?: string
  className?: string
  style?: CSSProperties
  ref?: Ref<ChatMarkdownEditorHandle>
}

export function ChatMarkdownEditor({
  initialValue,
  placeholder,
  ariaLabel,
  onChange,
  onKeyDown,
  onHeightChange,
  autoFocus,
  expanded,
  controls,
  className,
  style,
  ref,
}: ChatMarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement>(null)

  const keyHandlerRef = useRef(onKeyDown)
  keyHandlerRef.current = onKeyDown
  const keyPlugin = useMemo(
    () =>
      createPlatePlugin({
        key: 'agent-chat-keys',
        handlers: {
          onKeyDown: ({ editor: current, event }) => {
            if (
              (event.metaKey || event.ctrlKey) &&
              !event.shiftKey &&
              !event.altKey &&
              event.key.toLowerCase() === 'a'
            ) {
              const docStart = current.api.start([])
              const docEnd = current.api.end([])
              if (docStart && docEnd) {
                event.preventDefault()
                current.tf.select({ anchor: docStart, focus: docEnd })
              }
              return
            }
            const { selection } = current
            let atStart = false
            let atEnd = false
            if (selection && RangeApi.isCollapsed(selection)) {
              const docStart = current.api.start([])
              const docEnd = current.api.end([])
              atStart = !!docStart && PointApi.equals(selection.anchor, docStart)
              atEnd = !!docEnd && PointApi.equals(selection.anchor, docEnd)
            }
            keyHandlerRef.current(
              event as KeyboardEvent<HTMLDivElement>,
              () => chatValueToMarkdown(current.children as Value),
              { atStart, atEnd },
            )
          },
        },
      }),
    [],
  )

  const initial = useMemo(
    () => chatMarkdownToValue(initialValue),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount value only, deliberately not re-derived
    [],
  )

  const editor = usePlateEditor({
    plugins: [...chatComposerPlugins, keyPlugin],
    value: initial,
    autoSelect: 'end',
  })

  const handleChange = useCallback(() => {
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    onChange(chatValueToMarkdown(editor.children as Value))
  }, [editor, onChange])

  useImperativeHandle(
    ref,
    () => ({
      insertAttachmentMarkdown: (markdown: string) => {
        const nodes = chatMarkdownToValue(markdown)
        const at = editor.selection ?? editor.api.end([])
        editor.tf.insertNodes(nodes, { at, select: true })
        editor.tf.focus()
        onChange(chatValueToMarkdown(editor.children as Value))
      },
    }),
    [editor, onChange],
  )

  useLayoutEffect(() => {
    const host = hostRef.current
    const editable = host?.querySelector<HTMLElement>('[data-slate-editor]')
    if (!editable || !onHeightChange) return
    const report = () => onHeightChange(editable.getBoundingClientRect().height)
    report()
    const observer = new ResizeObserver(report)
    observer.observe(editable)
    return () => observer.disconnect()
  }, [onHeightChange])

  return (
    <Plate editor={editor} onChange={handleChange}>
      <div ref={hostRef} className="contents">
        <PlateContent
          autoFocus={autoFocus}
          placeholder={placeholder}
          aria-label={ariaLabel}
          aria-expanded={expanded}
          aria-controls={controls}
          className={cn('field', className)}
          style={style}
        />
      </div>
    </Plate>
  )
}
```

Edit `web/src/features/agent/composer/composer-field.tsx` to forward the ref and the new `wsId`/`chatId` props:

```tsx
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import {
  ChatMarkdownEditor,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { COMPOSER_LINE_HEIGHT } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerFieldProps {
  wsId: string
  chatId: string
  initialValue: string
  placeholder: string
  expanded: boolean
  controls?: string
  onChange: (value: string) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => void
  onHeightChange: (height: number) => void
  ref?: Ref<ChatMarkdownEditorHandle>
}

export function ComposerField({
  wsId,
  chatId,
  initialValue,
  placeholder,
  expanded,
  controls,
  onChange,
  onKeyDown,
  onHeightChange,
  ref,
}: ComposerFieldProps) {
  return (
    <ChatMarkdownEditor
      ref={ref}
      wsId={wsId}
      chatId={chatId}
      initialValue={initialValue}
      placeholder={placeholder}
      ariaLabel="Message the agent"
      expanded={expanded}
      controls={controls}
      autoFocus
      onChange={onChange}
      onKeyDown={onKeyDown}
      onHeightChange={onHeightChange}
      style={{ minHeight: COMPOSER_LINE_HEIGHT } satisfies CSSProperties}
    />
  )
}
```

Edit `web/src/features/agent/composer/agent-composer.tsx` to own the ref and pass `wsId`/`chatId` (add near the `modal` state added in Task 24):

```tsx
const editorRef = useRef<ChatMarkdownEditorHandle>(null)
```

and pass `ref={editorRef} wsId={props.wsId} chatId={props.chatId}` to `<ComposerField>` at `agent-composer.tsx:121`. (Add `useRef` and `type { ChatMarkdownEditorHandle }` imports.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/chat-markdown-editor.tsx web/src/features/agent/composer/composer-field.tsx web/src/features/agent/composer/agent-composer.tsx web/src/__tests__/features/agent/composer/plate/chat-markdown-editor.test.tsx
git commit -m "feat(composer): expose an imperative attachment-insert handle"
```

---

### Task 26: Attachment Markdown Builders (pure)

**Files:**
- Create: `web/src/features/agent/composer/lib/attachment-markdown.ts`
- Test: `web/src/__tests__/features/agent/composer/lib/attachment-markdown.test.ts`

**Interfaces:**
- Consumes: nothing (pure string builders)
- Produces: `textAttachmentMarkdown(id, text)`, `imageMarkdown(alt, ref)`, `fileMarkdown(filename, ref)`, `excalidrawMarkdown(id, sceneJson)` — consumed by Tasks 29, 31, 34

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/lib/attachment-markdown.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
  textAttachmentMarkdown,
  imageMarkdown,
  fileMarkdown,
  excalidrawMarkdown,
} from '@/features/agent/composer/lib/attachment-markdown'

describe('textAttachmentMarkdown', () => {
  it('fences the raw text with an id-suffixed language tag', () => {
    expect(textAttachmentMarkdown('abc123', 'hello\nworld')).toBe(
      '```text-attachment:abc123\nhello\nworld\n```',
    )
  })
})

describe('imageMarkdown', () => {
  it('produces a standard markdown image', () => {
    expect(imageMarkdown('diagram', 'chats/c1/attachments/x-a.png')).toBe(
      '![diagram](chats/c1/attachments/x-a.png)',
    )
  })
})

describe('fileMarkdown', () => {
  it('produces a standard markdown link', () => {
    expect(fileMarkdown('report.pdf', 'chats/c1/attachments/x-report.pdf')).toBe(
      '[report.pdf](chats/c1/attachments/x-report.pdf)',
    )
  })
})

describe('excalidrawMarkdown', () => {
  it('fences the scene JSON with an id-suffixed language tag', () => {
    expect(excalidrawMarkdown('abc123', '{"type":"excalidraw"}')).toBe(
      '```excalidraw:abc123\n{"type":"excalidraw"}\n```',
    )
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/attachment-markdown.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/lib/attachment-markdown.ts`:

```ts
/**
 * Markdown-string encodings for the four attachment kinds. Every editor-UX
 * insertion path (paste, drop, Attach File, Excalidraw save) builds one of
 * these and deserializes it through `chatMarkdownToValue` rather than
 * constructing Slate nodes directly — see chat-markdown-editor.tsx's
 * `insertAttachmentMarkdown`.
 *
 * The id suffix on the fenced kinds is load-bearing, not decorative: the
 * rendering-side plugin requires it before treating a `text-attachment`/
 * `excalidraw` fence as a real attachment rather than a code block that
 * merely mentions the tag (see the design spec's "fence-tag collision" test).
 */
export function textAttachmentMarkdown(id: string, text: string): string {
  return `\`\`\`text-attachment:${id}\n${text}\n\`\`\``
}

export function excalidrawMarkdown(id: string, sceneJson: string): string {
  return `\`\`\`excalidraw:${id}\n${sceneJson}\n\`\`\``
}

export function imageMarkdown(alt: string, ref: string): string {
  return `![${alt}](${ref})`
}

export function fileMarkdown(filename: string, ref: string): string {
  return `[${filename}](${ref})`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/attachment-markdown.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/lib/attachment-markdown.ts web/src/__tests__/features/agent/composer/lib/attachment-markdown.test.ts
git commit -m "feat(composer): add pure attachment markdown encoders"
```

---

### Task 27: `useTauriFileDrop` — the real desktop file-drop path

**Files:**
- Create: `web/src/features/file-system/lib/tauri-file-drop.ts`
- Test: `web/src/__tests__/features/file-system/lib/tauri-file-drop.test.ts`

**Interfaces:**
- Consumes: `isTauri()` (`@/lib/crowbar-bridge`), `@tauri-apps/api/webview`'s `getCurrentWebview().onDragDropEvent()` (dynamically imported, matching `daemon-health-listener.tsx`'s `await import('@tauri-apps/api/event')` precedent)
- Produces: `pointInRect(position, rect): boolean` (pure, tested directly), `useTauriFileDrop(containerRef, onDrop): void` — consumed by Tasks 28, 29, 30

This is exactly the case flagged as **genuinely hard to unit-test in isolation**: `useTauriFileDrop` wires a live Tauri IPC event to a live DOM ref, neither of which exists in a jsdom test environment, and `isTauri()` gates the whole body to a no-op outside Tauri. Only `pointInRect` gets a real unit test; the hook itself is verified live (Task 28's manual desktop verification note) and by the fact that `pointInRect`, its only non-trivial logic, is independently proven.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/file-system/lib/tauri-file-drop.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { pointInRect } from '@/features/file-system/lib/tauri-file-drop'

function rect(left: number, top: number, right: number, bottom: number): DOMRect {
  return { left, top, right, bottom, width: right - left, height: bottom - top, x: left, y: top, toJSON: () => ({}) }
}

describe('pointInRect', () => {
  it('is true for a point strictly inside', () => {
    expect(pointInRect({ x: 50, y: 50 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is true on the boundary (inclusive)', () => {
    expect(pointInRect({ x: 100, y: 100 }, rect(0, 0, 100, 100))).toBe(true)
    expect(pointInRect({ x: 0, y: 0 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is false outside the rect', () => {
    expect(pointInRect({ x: 101, y: 50 }, rect(0, 0, 100, 100))).toBe(false)
    expect(pointInRect({ x: 50, y: -1 }, rect(0, 0, 100, 100))).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/file-system/lib/tauri-file-drop.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/file-system/lib/tauri-file-drop.ts`:

```ts
import { useEffect } from 'react'
import type { RefObject } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'

export interface TauriDropPosition {
  x: number
  y: number
}

/** Pure — is `position` (Tauri's CSS-pixel, window-relative drop coordinate)
 *  inside `rect`. Inclusive of the boundary. */
export function pointInRect(position: TauriDropPosition, rect: DOMRect): boolean {
  return (
    position.x >= rect.left &&
    position.x <= rect.right &&
    position.y >= rect.top &&
    position.y <= rect.bottom
  )
}

/**
 * Real OS file paths dropped on `containerRef`'s element — Tauri desktop only.
 *
 * Tauri's webview intercepts a native OS file drag before it becomes a DOM
 * `drop` event at all: `dragDropEnabled` defaults to true and nothing in
 * `desktop/src-tauri/tauri.conf.json` overrides it, so `DataTransfer` never
 * carries files for a real drag and no DOM `drop` handler ever fires for
 * one. `extractDroppedFilePaths` (file-system-dropped-paths.ts) covers only
 * the other, unavoidably path-less case — a plain browser tab, where a
 * `DataTransfer` exists but no browser exposes a real host path on a `File`
 * either way. This hook is what actually answers "a real file got dropped
 * here, and here is its path" on the desktop build.
 *
 * Every mounted consumer receives the SAME window-wide event — Tauri does
 * not scope `onDragDropEvent` to an element — and filters it against its own
 * `getBoundingClientRect()`. Cheap, and avoids inventing a second routing
 * contract when the DOM already gives every consumer its own rect.
 */
export function useTauriFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  onDrop: (paths: string[]) => void,
): void {
  useEffect(() => {
    if (!isTauri()) return
    let disposed = false
    let unlisten: (() => void) | undefined

    void (async () => {
      const { getCurrentWebview } = await import('@tauri-apps/api/webview')
      const stop = await getCurrentWebview().onDragDropEvent((event) => {
        if (event.payload.type !== 'drop') return
        const el = containerRef.current
        if (!el) return
        if (pointInRect(event.payload.position, el.getBoundingClientRect())) {
          onDrop(event.payload.paths)
        }
      })
      if (disposed) stop()
      else unlisten = stop
    })()

    return () => {
      disposed = true
      unlisten?.()
    }
  }, [containerRef, onDrop])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/file-system/lib/tauri-file-drop.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/file-system/lib/tauri-file-drop.ts web/src/__tests__/features/file-system/lib/tauri-file-drop.test.ts
git commit -m "feat(file-system): add the real Tauri OS-file-drop path"
```

---

### Task 28: Revive the Dead Drop Call Sites (`pane-container.tsx`, `terminal.tsx`)

**Files:**
- Modify: `web/src/features/file-system/utils/file-system-dropped-paths.ts` (comment only — behavior unchanged, now honestly documented)
- Modify: `web/src/features/panes/components/pane-container.tsx`
- Modify: `web/src/features/terminal/components/terminal.tsx`
- Test: `web/src/__tests__/features/file-system/utils/file-system-dropped-paths.test.ts`

**Interfaces:**
- Consumes: `useTauriFileDrop` (Task 27)
- Produces: nothing new — restores real behavior at two existing call sites

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/file-system/utils/file-system-dropped-paths.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'

describe('extractDroppedFilePaths', () => {
  it('returns [] — a browser DataTransfer never carries a real host path; see useTauriFileDrop for where a real Tauri-desktop drop path comes from', () => {
    const dt = new DataTransfer()
    expect(extractDroppedFilePaths(dt)).toEqual([])
  })
})
```

(This test already passes today — it's here to lock the documented, intentional behavior in place so a future edit doesn't quietly "fix" it into something that can't actually work.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/file-system/utils/file-system-dropped-paths.test.ts`
Expected: PASS already (this step is a no-op sanity check, not a red-first step — see note below).

*Note on TDD shape here*: this task's actual change is in `pane-container.tsx`/`terminal.tsx`, not in `file-system-dropped-paths.ts` itself (which stays `[]`, now honestly commented rather than stubbed). There is no meaningful pure-function red state to chase for the drag-drop wiring itself — it is DOM/Tauri-IPC integration, the same "hard to unit-test" category as Task 27's hook. The regression-locking test above is the practical substitute.

- [ ] **Step 3: Write minimal implementation**

Edit `web/src/features/file-system/utils/file-system-dropped-paths.ts`:

```ts
/**
 * A browser `DataTransfer` never carries a real host filesystem path — no
 * browser exposes one on a `File`, dev-mode or not. On the Tauri desktop
 * build this function is not even reachable for a real OS file drop: Tauri's
 * webview intercepts that before a DOM `drop` event fires at all (see
 * `useTauriFileDrop`, `features/file-system/lib/tauri-file-drop.ts`, for the
 * real path-yielding mechanism). This stays `[]` because that is the honest
 * answer for what a `DataTransfer` alone can ever provide, not because the
 * feature is unimplemented — the real implementation lives in the hook.
 */
export function extractDroppedFilePaths(_dataTransfer: DataTransfer): string[] {
  return []
}
```

Edit `web/src/features/panes/components/pane-container.tsx` — add alongside the existing `handleDrop` (near `pane-container.tsx:421-444`; `containerRef` already exists at `pane-container.tsx:517`):

```tsx
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
```

```tsx
  const handleTauriFileDrop = useCallback(
    async (paths: string[]) => {
      if (paths.length === 0 || !handleFileOpen) return
      workspaceStore.getState().paneActions.setActivePane(pane.id)
      for (const droppedPath of paths) {
        await handleFileOpen(droppedPath, false)
      }
    },
    [pane.id, handleFileOpen, workspaceStore],
  )
  useTauriFileDrop(containerRef, handleTauriFileDrop)
```

Edit `web/src/features/terminal/components/terminal.tsx` — add alongside `handleTerminalFileDrop` (near `terminal.tsx:474-486`; `terminalContainerRef` already exists at `terminal.tsx:1437`):

```tsx
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
```

```tsx
  const handleTauriTerminalDrop = useCallback(
    (paths: string[]) => {
      const text = formatDroppedPathsForTerminal(paths)
      if (!text) return
      writeBuffered(text, 'file-drop')
      requestAnimationFrame(() => xtermRef.current?.focus())
    },
    [writeBuffered],
  )
  useTauriFileDrop(terminalContainerRef, handleTauriTerminalDrop)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/file-system/utils/file-system-dropped-paths.test.ts`
Expected: PASS (unchanged, now with the derived rationale checked in). Manual verification of the real Tauri path (dragging a Finder file onto an editor pane / terminal in `make dev-desktop`) is required before calling this task done — this is exactly the "hard to unit-test" surface, per the memory on verifying via dev-desktop rather than headless.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/file-system/utils/file-system-dropped-paths.ts web/src/features/panes/components/pane-container.tsx web/src/features/terminal/components/terminal.tsx web/src/__tests__/features/file-system/utils/file-system-dropped-paths.test.ts
git commit -m "fix(file-system): revive real file-drop paths via Tauri's drag-drop event"
```

---

### Task 29: Composer Drag-and-Drop Wiring

**Files:**
- Modify: `web/src/features/agent/composer/agent-composer.tsx`
- Modify: `web/src/features/agent/styles/composer.css`
- Test: `web/src/__tests__/features/agent/composer/agent-composer.test.tsx`

**Interfaces:**
- Consumes: `useTauriFileDrop` (Task 27), `insertAttachmentMarkdown` (Task 25, via `editorRef`), `fileMarkdown`/`imageMarkdown` (Task 26), `uploadChatAttachment` (Task 20)
- Produces: dropping a file directly on the composer pill inserts the same attachment node the Attach File modal (Task 30) would

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/agent-composer.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AgentComposer } from '@/features/agent/composer/agent-composer'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async () => ({
    ref: 'chats/c1/attachments/x-a.png',
    filename: 'a.png',
    size: 10,
    contentType: 'image/png',
  })),
}))

afterEach(cleanup)

const baseProps = {
  wsId: 'w1',
  chatId: 'c1',
  activity: 'idle' as const,
  providerLabel: 'Claude',
  live: true,
  working: false,
  compacting: false,
  sending: false,
  submitUnavailable: false,
  canStop: false,
  draft: '',
  fieldHeight: 20,
  slashOpen: false,
  onDraftChange: vi.fn(),
  onHeightChange: vi.fn(),
  onKeyDown: vi.fn(),
  onSend: vi.fn(),
  onStop: vi.fn(),
  onOpenTerminal: vi.fn(),
  draftSeed: 0,
  seedText: '',
}

describe('AgentComposer drag-and-drop', () => {
  it('shows a drag-over state while a file is dragged over the pill', () => {
    render(<AgentComposer {...baseProps} />)
    const pill = screen.getByRole('textbox', { name: /message the agent/i }).closest('.pill')
    fireEvent.dragOver(pill!, { dataTransfer: { types: ['Files'] } })
    expect(pill).toHaveClass('drop-target')
    fireEvent.dragLeave(pill!)
    expect(pill).not.toHaveClass('drop-target')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/agent-composer.test.tsx`
Expected: FAIL — `.pill` has no `onDragOver`/`drop-target` state yet.

- [ ] **Step 3: Write minimal implementation**

Edit `web/src/features/agent/composer/agent-composer.tsx` (extends the `modal`/`editorRef` state from Tasks 24–25):

```tsx
import { useCallback, useRef, useState } from 'react'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { fileMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
```

```tsx
  const pillRef = useRef<HTMLDivElement>(null)
  const [dropTarget, setDropTarget] = useState(false)

  const insertUploaded = useCallback(
    (result: { ref: string; filename: string; contentType: string }) => {
      const md = result.contentType.startsWith('image/')
        ? imageMarkdown(result.filename, result.ref)
        : fileMarkdown(result.filename, result.ref)
      editorRef.current?.insertAttachmentMarkdown(md)
    },
    [],
  )

  const uploadAndInsert = useCallback(
    async (input: { file: File } | { path: string }) => {
      const result = await uploadChatAttachment(props.wsId, props.chatId, input)
      insertUploaded(result)
    },
    [props.wsId, props.chatId, insertUploaded],
  )

  useTauriFileDrop(pillRef, (paths) => {
    setDropTarget(false)
    for (const path of paths) void uploadAndInsert({ path })
  })

  const handlePillDragOver = useCallback((e: React.DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    setDropTarget(true)
  }, [])

  const handlePillDragLeave = useCallback((e: React.DragEvent) => {
    const related = e.relatedTarget as HTMLElement | null
    if (!related || !e.currentTarget.contains(related)) setDropTarget(false)
  }, [])

  // Plain-browser (non-Tauri dev) fallback: a real DataTransfer.files DOES
  // carry usable File bytes here — this is a completely different problem
  // from extractDroppedFilePaths's (which is about a host PATH, not bytes).
  const handlePillDrop = useCallback(
    (e: React.DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [uploadAndInsert],
  )
```

And on the `.pill` div (`agent-composer.tsx:120`):

```tsx
        <div
          ref={pillRef}
          className={cn('pill', isMultiline(props.fieldHeight) && 'multi', dropTarget && 'drop-target')}
          onDragOver={handlePillDragOver}
          onDragLeave={handlePillDragLeave}
          onDrop={handlePillDrop}
        >
```

Append to `web/src/features/agent/styles/composer.css`:

```css
.agent-chat .pill.drop-target {
  border-color: color-mix(in oklch, var(--ring) 60%, transparent);
  box-shadow:
    0 1px 2px oklch(0 0 0 / 0.06),
    0 0 0 3px color-mix(in oklch, var(--ring) 25%, transparent);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/agent-composer.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/agent-composer.tsx web/src/features/agent/styles/composer.css web/src/__tests__/features/agent/composer/agent-composer.test.tsx
git commit -m "feat(composer): drop a file directly on the composer to attach it"
```

---

### Task 30: Attach File Modal

**Files:**
- Create: `web/src/features/agent/composer/attach-file-modal.tsx`
- Modify: `web/src/features/agent/composer/agent-composer.tsx` (replaces `modal === 'attach-file'` placeholder from Task 24)
- Test: `web/src/__tests__/features/agent/composer/attach-file-modal.test.tsx`

**Interfaces:**
- Consumes: `uploadChatAttachment` (Task 20), `useTauriFileDrop` (Task 27), `fileMarkdown`/`imageMarkdown` (Task 26), `toast` (`@/features/window/stores/toast-store`)
- Produces: `AttachFileModal({ wsId, chatId, open, onClose, onInsertMarkdown: (md: string) => void })`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/attach-file-modal.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async (_ws: string, _chat: string, input: { file: File } | { path: string }) => ({
    ref: 'path' in input ? `chats/c1/attachments/x-${input.path}` : `chats/c1/attachments/x-${input.file.name}`,
    filename: 'path' in input ? input.path : input.file.name,
    size: 10,
    contentType: 'file' in input ? input.file.type || 'application/octet-stream' : 'application/octet-stream',
  })),
}))

afterEach(cleanup)

describe('AttachFileModal', () => {
  it('uploads a browsed file and inserts a file-link markdown node', async () => {
    const onInsertMarkdown = vi.fn()
    render(
      <AttachFileModal wsId="w1" chatId="c1" open onClose={vi.fn()} onInsertMarkdown={onInsertMarkdown} />,
    )
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() =>
      expect(onInsertMarkdown).toHaveBeenCalledWith('[notes.txt](chats/c1/attachments/x-notes.txt)'),
    )
  })

  it('inserts an image node for an image file', async () => {
    const onInsertMarkdown = vi.fn()
    render(
      <AttachFileModal wsId="w1" chatId="c1" open onClose={vi.fn()} onInsertMarkdown={onInsertMarkdown} />,
    )
    const input = screen.getByLabelText(/choose a file/i) as HTMLInputElement
    const file = new File(['x'], 'shot.png', { type: 'image/png' })
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() =>
      expect(onInsertMarkdown).toHaveBeenCalledWith('![shot.png](chats/c1/attachments/x-shot.png)'),
    )
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/attach-file-modal.test.tsx`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/attach-file-modal.tsx`:

```tsx
import { useCallback, useRef, useState } from 'react'
import { Dialog, DialogPopup, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { fileMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { toast } from '@/features/window/stores/toast-store'
import { cn } from '@/lib/utils'

interface AttachFileModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

/**
 * A single entry point regardless of kind — image, CSV, PDF, other — the
 * kind is inferred from the uploaded file's own contentType, not from
 * anything the user picked here.
 *
 * Modal, not inline: the composer pill has no room to grow a picker without
 * shoving the transcript around mid-drag — see excalidraw-modal.tsx for the
 * same tradeoff made the same way.
 */
export function AttachFileModal({ wsId, chatId, open, onClose, onInsertMarkdown }: AttachFileModalProps) {
  const [dropTarget, setDropTarget] = useState(false)
  const [uploading, setUploading] = useState(false)
  const dropzoneRef = useRef<HTMLDivElement>(null)

  const uploadAndInsert = useCallback(
    async (input: { file: File } | { path: string }) => {
      setUploading(true)
      try {
        const result = await uploadChatAttachment(wsId, chatId, input)
        const md = result.contentType.startsWith('image/')
          ? imageMarkdown(result.filename, result.ref)
          : fileMarkdown(result.filename, result.ref)
        onInsertMarkdown(md)
        onClose()
      } catch (err) {
        toast.error("Couldn't attach that file", err instanceof Error ? err.message : String(err))
      } finally {
        setUploading(false)
      }
    },
    [wsId, chatId, onInsertMarkdown, onClose],
  )

  useTauriFileDrop(dropzoneRef, (paths) => {
    setDropTarget(false)
    for (const path of paths) void uploadAndInsert({ path })
  })

  const handleDragOver = useCallback((e: React.DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    setDropTarget(true)
  }, [])

  const handleDragLeave = useCallback(() => setDropTarget(false), [])

  // Plain-browser fallback — see agent-composer.tsx's own note on why this is
  // unrelated to extractDroppedFilePaths (bytes here, not a host path).
  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [uploadAndInsert],
  )

  const handleBrowse = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      for (const file of Array.from(e.target.files ?? [])) void uploadAndInsert({ file })
      e.target.value = ''
    },
    [uploadAndInsert],
  )

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Attach a file</DialogTitle>
          <DialogDescription>Drop it here, or choose one from your computer.</DialogDescription>
        </DialogHeader>
        <div
          ref={dropzoneRef}
          className={cn('dropzone', dropTarget && 'drop-target')}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <p>{uploading ? 'Uploading…' : 'Drag a file here'}</p>
          <Button asChild variant="outline" disabled={uploading}>
            <label>
              Choose a file
              <input
                type="file"
                aria-label="Choose a file"
                className="sr-only"
                disabled={uploading}
                onChange={handleBrowse}
              />
            </label>
          </Button>
        </div>
      </DialogPopup>
    </Dialog>
  )
}
```

Edit `web/src/features/agent/composer/agent-composer.tsx`, replacing the Task 24 placeholder:

```tsx
        {modal === 'attach-file' && (
          <AttachFileModal
            wsId={props.wsId}
            chatId={props.chatId}
            open
            onClose={() => setModal(null)}
            onInsertMarkdown={(md) => editorRef.current?.insertAttachmentMarkdown(md)}
          />
        )}
```

(add `import { AttachFileModal } from '@/features/agent/composer/attach-file-modal'`)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/attach-file-modal.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/attach-file-modal.tsx web/src/features/agent/composer/agent-composer.tsx web/src/__tests__/features/agent/composer/attach-file-modal.test.tsx
git commit -m "feat(composer): add the Attach File picker modal"
```

---

### Task 31: Paste Threshold + Shift-State Tracking (pure logic)

**Files:**
- Create: `web/src/features/agent/composer/lib/paste-threshold.ts`
- Test: `web/src/__tests__/features/agent/composer/lib/paste-threshold.test.ts`

**Interfaces:**
- Consumes: nothing
- Produces: `PASTE_CHAR_THRESHOLD`, `PASTE_LINE_THRESHOLD`, `shouldWrapAsTextAttachment(text: string): boolean` — consumed by Task 32

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/lib/paste-threshold.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
  shouldWrapAsTextAttachment,
  PASTE_CHAR_THRESHOLD,
  PASTE_LINE_THRESHOLD,
} from '@/features/agent/composer/lib/paste-threshold'

describe('shouldWrapAsTextAttachment', () => {
  it('stays plain text under both thresholds', () => {
    expect(shouldWrapAsTextAttachment('a short line')).toBe(false)
  })

  it('wraps once the char threshold is exceeded', () => {
    expect(shouldWrapAsTextAttachment('a'.repeat(PASTE_CHAR_THRESHOLD))).toBe(false)
    expect(shouldWrapAsTextAttachment('a'.repeat(PASTE_CHAR_THRESHOLD + 1))).toBe(true)
  })

  it('wraps once the line threshold is exceeded, even if short', () => {
    const text = Array.from({ length: PASTE_LINE_THRESHOLD + 1 }, () => 'x').join('\n')
    expect(shouldWrapAsTextAttachment(text)).toBe(true)
  })

  it('does not wrap at exactly the line threshold', () => {
    const text = Array.from({ length: PASTE_LINE_THRESHOLD }, () => 'x').join('\n')
    expect(shouldWrapAsTextAttachment(text)).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/paste-threshold.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/lib/paste-threshold.ts`:

```ts
/**
 * Paste-to-pill threshold — the design spec's own proposed default, adopted
 * as-is: long enough that a normal sentence or short snippet never collapses
 * (nobody wants "here's the fix:" turning into a pill), short enough that a
 * genuinely large paste — a stack trace, a file dump — collapses instead of
 * flooding the box as unreadable wrapped prose. Tunable, not architectural;
 * see the design spec's "Thresholds & fallback rules".
 */
export const PASTE_CHAR_THRESHOLD = 400
export const PASTE_LINE_THRESHOLD = 6

/** Either threshold alone is enough — a long single line (a minified blob,
 *  a URL-encoded token) is exactly as unreadable inline as six short ones. */
export function shouldWrapAsTextAttachment(text: string): boolean {
  if (text.length > PASTE_CHAR_THRESHOLD) return true
  const lineCount = text.split('\n').length
  return lineCount > PASTE_LINE_THRESHOLD
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/lib/paste-threshold.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/lib/paste-threshold.ts web/src/__tests__/features/agent/composer/lib/paste-threshold.test.ts
git commit -m "feat(composer): add paste-to-text-attachment threshold logic"
```

---

### Task 32: Paste Interception Plate Plugin

**Files:**
- Create: `web/src/features/agent/composer/plate/chat-paste-plugin.ts`
- Modify: `web/src/features/agent/composer/plate/chat-markdown-editor.tsx`
- Modify: `web/src/features/agent/composer/composer-field.tsx`
- Modify: `web/src/features/agent/composer/agent-composer.tsx`
- Modify: `web/src/features/agent/chat/agent-empty-document.tsx`
- Modify: `web/src/features/agent/chat/agent-chat-view.tsx`
- Test: `web/src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx`

**Interfaces:**
- Consumes: `shouldWrapAsTextAttachment` (Task 31), `textAttachmentMarkdown`/`imageMarkdown` (Task 26), `uploadChatAttachment` (Task 20), `CodeBlockPlugin` (`@platejs/code-block/react`, already registered in `chatComposerPlugins`)
- Produces: paste interception wired into every `ChatMarkdownEditor` instance (composer pill AND blank-document surface, for free — both consume the same component)

Confirmed by search (`grep -rn "createPlatePlugin(" web/src`): no `onPaste`/`onDrop` plugin handler exists anywhere in this codebase today, and `LinkRules.autolink({variant:'paste'})` (`link-kit.tsx:13`) is a Link-plugin-internal input rule, not a general paste interceptor — this is genuinely the first one.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ChatMarkdownEditor,
} from '@/features/agent/composer/plate/chat-markdown-editor'

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async () => ({
    ref: 'chats/c1/attachments/x-pasted-image.png',
    filename: 'pasted-image.png',
    size: 10,
    contentType: 'image/png',
  })),
}))

afterEach(cleanup)

function paste(el: HTMLElement, data: { text?: string; files?: File[]; shift?: boolean }) {
  if (data.shift) fireEvent.keyDown(el, { key: 'Shift', shiftKey: true })
  const items = (data.files ?? []).map((file) => ({
    type: file.type,
    kind: 'file',
    getAsFile: () => file,
  }))
  fireEvent.paste(el, {
    clipboardData: {
      getData: () => data.text ?? '',
      items,
      types: data.text ? ['text/plain'] : ['Files'],
    },
  })
  if (data.shift) fireEvent.keyUp(el, { key: 'Shift', shiftKey: false })
}

describe('chat paste interception', () => {
  it('wraps an over-threshold plain-text paste as a text-attachment fence', async () => {
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        wsId="w1"
        chatId="c1"
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )
    const editable = screen.getByRole('textbox', { name: /message the agent/i })
    paste(editable, { text: 'x'.repeat(500) })

    await waitFor(() => {
      const last = onChange.mock.calls.at(-1)?.[0] as string
      expect(last).toContain('```text-attachment:')
    })
  })

  it('lets Shift+paste bypass interception entirely', async () => {
    const onChange = vi.fn()
    render(
      <ChatMarkdownEditor
        wsId="w1"
        chatId="c1"
        initialValue=""
        placeholder=""
        ariaLabel="Message the agent"
        onChange={onChange}
        onKeyDown={vi.fn()}
      />,
    )
    const editable = screen.getByRole('textbox', { name: /message the agent/i })
    paste(editable, { text: 'x'.repeat(500), shift: true })

    await waitFor(() => {
      const last = onChange.mock.calls.at(-1)?.[0] as string
      expect(last).not.toContain('text-attachment')
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx`
Expected: FAIL — no paste interception registered yet.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/plate/chat-paste-plugin.ts`:

```ts
import { createPlatePlugin } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { MarkdownPlugin } from '@platejs/markdown'
import { shouldWrapAsTextAttachment } from '@/features/agent/composer/lib/paste-threshold'
import { textAttachmentMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { nanoid } from 'nanoid'

interface ChatPastePluginOptions {
  wsId: string
  chatId: string
}

/**
 * Paste interception, as a plugin's `handlers` — NOT the `PlateContent` DOM
 * prop. Same reasoning as `agent-chat-keys`'s onKeyDown (chat-markdown-
 * editor.tsx): a DOM `onPaste` prop fires after Slate's own default paste
 * insertion has already run, too late to `preventDefault()` it.
 *
 * Order, per the design spec's "Editor UX":
 *  1. Caret inside a code block -> do nothing, let default paste happen.
 *  2. Shift held at paste time -> bypass everything, default paste happens.
 *     The browser `paste` event carries no modifier info of its own, so
 *     Shift is tracked separately via this SAME plugin's onKeyDown/onKeyUp
 *     — deliberately independent of `agent-chat-keys`, which owns Enter/
 *     Cmd+A and has nothing to do with paste.
 *  3. Clipboard has image data -> always intercepted, uploads + inserts an
 *     image node.
 *  4. Plain text over threshold -> wrapped as a `text-attachment` fence.
 *  5. Otherwise -> default paste happens (short plain text).
 */
export function createChatPastePlugin({ wsId, chatId }: ChatPastePluginOptions) {
  let shiftHeld = false

  return createPlatePlugin({
    key: 'agent-chat-paste',
    handlers: {
      onKeyDown: ({ event }) => {
        if (event.key === 'Shift') shiftHeld = true
      },
      onKeyUp: ({ event }) => {
        if (event.key === 'Shift') shiftHeld = false
      },
      onPaste: ({ editor, event }) => {
        if (shiftHeld) return

        const inCodeBlock = editor.api.above({ match: { type: CodeBlockPlugin.key } })
        if (inCodeBlock) return

        const clipboard = event.clipboardData
        const imageItem = Array.from(clipboard?.items ?? []).find((item) =>
          item.type.startsWith('image/'),
        )
        if (imageItem) {
          event.preventDefault()
          const file = imageItem.getAsFile()
          if (!file) return
          const at = editor.selection ?? editor.api.end([])
          void uploadChatAttachment(wsId, chatId, { file }).then((result) => {
            const nodes = editor.getApi(MarkdownPlugin).markdown.deserialize(
              imageMarkdown(result.filename, result.ref),
            )
            editor.tf.insertNodes(nodes, { at, select: true })
          })
          return
        }

        const text = clipboard?.getData('text/plain') ?? ''
        if (!shouldWrapAsTextAttachment(text)) return

        event.preventDefault()
        const id = nanoid()
        const nodes = editor.getApi(MarkdownPlugin).markdown.deserialize(
          textAttachmentMarkdown(id, text),
        )
        editor.tf.insertNodes(nodes, { at: editor.selection ?? editor.api.end([]), select: true })
      },
    },
  })
}
```

Edit `web/src/features/agent/composer/plate/chat-markdown-editor.tsx` — register the plugin (`wsId`/`chatId` are already accepted props since Task 25):

```tsx
import { createChatPastePlugin } from '@/features/agent/composer/plate/chat-paste-plugin'
```

```tsx
export function ChatMarkdownEditor({
  wsId,
  chatId,
  initialValue,
  // ...unchanged destructure...
  ref,
}: ChatMarkdownEditorProps) {
  // ...unchanged keyPlugin...
  const pastePlugin = useMemo(() => createChatPastePlugin({ wsId, chatId }), [wsId, chatId])
  // ...
  const editor = usePlateEditor({
    plugins: [...chatComposerPlugins, keyPlugin, pastePlugin],
    value: initial,
    autoSelect: 'end',
  })
  // ...rest unchanged...
```

Edit `web/src/features/agent/chat/agent-empty-document.tsx` — add `wsId`/`chatId` to `AgentEmptyDocumentProps` and pass through to its own `<ChatMarkdownEditor>` (`agent-empty-document.tsx:147`), gaining paste interception on the blank-chat surface for free (same shared component).

Edit `web/src/features/agent/chat/agent-chat-view.tsx:721` — pass `wsId={wsId} chatId={chatId}` into `<AgentEmptyDocument>`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/chat-paste-plugin.ts web/src/features/agent/composer/plate/chat-markdown-editor.tsx web/src/features/agent/composer/composer-field.tsx web/src/features/agent/composer/agent-composer.tsx web/src/features/agent/chat/agent-empty-document.tsx web/src/features/agent/chat/agent-chat-view.tsx web/src/__tests__/features/agent/composer/plate/chat-paste-plugin.test.tsx
git commit -m "feat(composer): intercept paste for text-attachment/image thresholds"
```

---

### Task 33: Attachment Block Drag-Handle Primitive

**Files:**
- Create: `web/src/features/agent/composer/plate/attachment-drag-handle.tsx`
- Test: `web/src/__tests__/features/agent/composer/plate/attachment-drag-handle.test.tsx`

**Interfaces:**
- Consumes: `useDraggable`/`useDropLine` (`@platejs/dnd`), `PathApi` (`platejs`), `useEditorRef` (`platejs/react`) — the SAME primitives `table-node.tsx`'s `RowDragHandle` already uses; NOT `BlockMenuKit` (confirmed to contain no drag primitive in this codebase — see the architectural findings above)
- Produces: `useAttachmentDraggable(element): { isDragging, nodeRef, previewRef, handleRef }`, `AttachmentDragHandle({ dragRef, onSelect })`, `AttachmentDropLine()` — **this is the interface boundary the Phase 4 integration task wires into Phase 2's node components**: wrap a node's root element with `ref={useComposedRef(props.ref, previewRef, nodeRef)}` and `className={cn('group/attachment', isDragging && 'opacity-50')}`, and render `<AttachmentDragHandle dragRef={handleRef} />` + `<AttachmentDropLine />` inside

This is genuinely hard to unit-test past its pure boundary: `useDraggable` wires HTML5 drag events, pointer capture, and Slate transforms together, none of which a jsdom `render()` exercises meaningfully. The test below exercises only what's real to check in isolation — the handle renders, exposes the grip affordance by role, and calls the provided `onSelect` on click (the one plain synchronous side effect `RowDragHandle` demonstrates is safe to assert). Actual reorder-by-drag needs the same "hard to unit-test — verify live" treatment as Tasks 27/28/29's Tauri drop wiring.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/plate/attachment-drag-handle.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Plate, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { AttachmentDragHandle } from '@/features/agent/composer/plate/attachment-drag-handle'

afterEach(cleanup)

function Harness() {
  const editor = usePlateEditor({
    plugins: chatComposerPlugins,
    value: [{ type: 'p', children: [{ text: 'hi' }] }],
  })
  return (
    <Plate editor={editor}>
      <AttachmentDragHandle dragRef={null} onSelect={() => editor.tf.select(editor.children[0]!)} />
    </Plate>
  )
}

describe('AttachmentDragHandle', () => {
  it('renders a grab affordance', () => {
    render(<Harness />)
    expect(screen.getByRole('button', { name: /reorder/i })).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachment-drag-handle.test.tsx`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/features/agent/composer/plate/attachment-drag-handle.tsx`:

```tsx
import { GripVertical } from 'lucide-react'
import { useDraggable, useDropLine } from '@platejs/dnd'
import { PathApi, type TElement } from 'platejs'
import { useEditorRef } from 'platejs/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/**
 * `BlockMenuKit` (components/editor/plugins/block-menu-kit.tsx) does NOT
 * carry a drag-to-reorder handle in this codebase — its own upstream
 * template comment names a `dnd-kit.tsx` (block-selection-kit.tsx:6) that
 * was never actually added here; `BlockMenuKit` is block SELECTION plus a
 * right-click context menu only. The only real drag-reorder primitive that
 * exists in this repo is `@platejs/dnd`'s `useDraggable`/`useDropLine`,
 * already used for table-row reordering (components/ui/table-node.tsx:
 * 1087-1187) — this module is that same primitive, scoped to a single
 * attachment block instead of a table row, with none of `BlockMenuKit`'s
 * would-be `/`-insert or block-type-conversion chrome (chat deliberately
 * has neither).
 */
export function useAttachmentDraggable(element: TElement) {
  const editor = useEditorRef()
  return useDraggable({
    element,
    type: element.type,
    // Attachments reorder only among their own siblings at the SAME level —
    // not into a list item or a table cell, mirroring the table row's own
    // same-parent constraint.
    canDropNode: ({ dragEntry, dropEntry }) =>
      PathApi.equals(PathApi.parent(dragEntry[1]), PathApi.parent(dropEntry[1])),
    onDropHandler: (_, { dragItem }) => {
      const dragElement = (dragItem as { element: TElement }).element
      if (dragElement) editor.tf.select(dragElement)
    },
  })
}

export function AttachmentDragHandle({
  dragRef,
  onSelect,
}: {
  dragRef: React.Ref<HTMLButtonElement> | null
  onSelect?: () => void
}) {
  return (
    <Button
      ref={dragRef ?? undefined}
      variant="outline"
      aria-label="Reorder this attachment"
      className={cn(
        '-translate-y-1/2 absolute top-1/2 left-0 z-51 h-6 w-4 p-0 focus-visible:ring-0 focus-visible:ring-offset-0',
        'cursor-grab active:cursor-grabbing',
        'opacity-0 transition-opacity duration-100 group-hover/attachment:opacity-100',
      )}
      onClick={onSelect}
    >
      <GripVertical className="text-muted-foreground" />
    </Button>
  )
}

export function AttachmentDropLine() {
  const { dropLine } = useDropLine()
  if (!dropLine) return null
  return (
    <div
      className={cn(
        'absolute inset-x-0 left-2 z-50 h-0.5 bg-brand/50',
        dropLine === 'top' ? '-top-px' : '-bottom-px',
      )}
    />
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachment-drag-handle.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachment-drag-handle.tsx web/src/__tests__/features/agent/composer/plate/attachment-drag-handle.test.tsx
git commit -m "feat(composer): add a standalone drag-to-reorder handle for attachment blocks"
```

---

### Task 34: Excalidraw Dependency + Embedded Drawing Editor Modal

**Files:**
- Modify: `web/package.json` (add `@excalidraw/excalidraw`)
- Create: `web/src/features/agent/composer/excalidraw-modal.tsx` (light chrome, always in the main bundle)
- Create: `web/src/features/agent/composer/excalidraw-canvas.tsx` (the actual `<Excalidraw>` mount — lazy-loaded)
- Modify: `web/src/features/agent/composer/agent-composer.tsx` (replaces `modal === 'excalidraw'` placeholder from Task 24)
- Test: `web/src/__tests__/features/agent/composer/excalidraw-modal.test.tsx`

**Interfaces:**
- Consumes: `@excalidraw/excalidraw`'s `Excalidraw` component and `exportToBlob` (new dependency), `uploadChatAttachment` (Task 20), `excalidrawMarkdown`/`imageMarkdown` (Task 26)
- Produces: `ExcalidrawModal({ wsId, chatId, open, onClose, onInsertMarkdown: (md: string) => void })` — inserts BOTH the fenced JSON block and the sibling `![diagram](ref)` image, per the spec's encoding, using **one shared id** for both

Confirmed absent from `web/package.json` (`grep -n excalidraw web/package.json` → no match); this task adds it.

Modal, not an inline panel: the composer pill is a single 20-38px-tall line that grows with text — there is no stable, non-jank spot to dock a full drawing canvas beside it without the pill's own `handleOffset`/`isMultiline` geometry (Task 21) fighting the canvas for space on every keystroke. `AttachFileModal` (Task 30) already established the modal precedent for exactly this "composer has no room" reason.

`@excalidraw/excalidraw` is a large, React-tree-mounting library (comparable to or larger than the katex bundle `chat-composer-plugins.ts` explicitly keeps OUT of the composer's own module scope — see that file's comment on `MathKit`). It must not load eagerly with the composer; `excalidraw-canvas.tsx` is lazy-loaded exactly like `MarkdownEditorPane` (`features/panes/components/editor-pane.tsx:12-13`, `lazy(() => import(...).then((m) => ({ default: m.X })))` + `<Suspense>`).

**Critical detail this task must get right**: the fence-tag id (Task 26's `excalidrawMarkdown(id, sceneJson)`) and the filename shortid the PNG gets uploaded under must be the SAME value — that's the whole point of the design spec's resolved open question ("the client mints the id and passes it through"). `uploadChatAttachment` (Task 20) accepts an optional 4th `id` argument for exactly this; this task's `handleSave` must generate ONE `nanoid()` and pass it to **both** `excalidrawMarkdown(id, ...)` and `uploadChatAttachment(wsId, chatId, { file: pngFile }, id)` — not let the upload mint its own.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/agent/composer/excalidraw-modal.test.tsx`:

```tsx
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExcalidrawModal } from '@/features/agent/composer/excalidraw-modal'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'

const save = vi.fn()
vi.mock('@/features/agent/composer/excalidraw-canvas', () => ({
  ExcalidrawCanvas: ({ onSave }: { onSave: (fn: typeof save) => void }) => {
    onSave(save)
    return <div data-testid="excalidraw-canvas" />
  },
}))

vi.mock('@/features/agent/api/upload-chat-attachment', () => ({
  uploadChatAttachment: vi.fn(async () => ({
    ref: 'chats/c1/attachments/x-diagram.png',
    filename: 'x-diagram.png',
    size: 10,
    contentType: 'image/png',
  })),
}))

afterEach(cleanup)

describe('ExcalidrawModal', () => {
  it('lazily mounts the canvas only once opened', async () => {
    render(
      <ExcalidrawModal wsId="w1" chatId="c1" open onClose={vi.fn()} onInsertMarkdown={vi.fn()} />,
    )
    await waitFor(() => expect(screen.getByTestId('excalidraw-canvas')).toBeInTheDocument())
  })

  it('uses the SAME id for the fence-tag JSON and the uploaded PNG filename', async () => {
    const onInsertMarkdown = vi.fn()
    render(
      <ExcalidrawModal wsId="w1" chatId="c1" open onClose={vi.fn()} onInsertMarkdown={onInsertMarkdown} />,
    )
    await waitFor(() => expect(screen.getByTestId('excalidraw-canvas')).toBeInTheDocument())

    const pngFile = new File(['x'], 'diagram.png', { type: 'image/png' })
    await save.mock.calls.length // no-op await to satisfy lint; real call happens below
    const onSaveCallback = (
      vi.mocked(uploadChatAttachment).mock.calls.length,
      screen.getByTestId('excalidraw-canvas')
    )
    void onSaveCallback

    // Simulate the canvas calling back with a scene + PNG.
    const props = vi.mocked(uploadChatAttachment)
    // The component under test wires `save` (above) to its own handleSave —
    // invoke it directly, as excalidraw-canvas.tsx would on a real Save click.
    await save({ sceneJson: '{"elements":[],"appState":{}}', pngFile } as never)

    expect(props).toHaveBeenCalledTimes(1)
    const [, , , uploadedId] = props.mock.calls[0]
    const [fenceMarkdown] = onInsertMarkdown.mock.calls[0]
    expect(fenceMarkdown).toContain(`excalidraw:${uploadedId}`)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/excalidraw-modal.test.tsx`
Expected: FAIL — module does not exist, `@excalidraw/excalidraw` not installed.

- [ ] **Step 3: Write minimal implementation**

```bash
cd web && bun add @excalidraw/excalidraw
```

Create `web/src/features/agent/composer/excalidraw-canvas.tsx`:

```tsx
import { useCallback, useState } from 'react'
import { Excalidraw, exportToBlob } from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'
import type { ExcalidrawImperativeAPI } from '@excalidraw/excalidraw/types'
import { Button } from '@/components/ui/button'

export interface ExcalidrawSaveResult {
  sceneJson: string
  pngFile: File
}

interface ExcalidrawCanvasProps {
  onCancel: () => void
  onSave: (result: ExcalidrawSaveResult) => void
}

/** The actual heavy mount — see excalidraw-modal.tsx for why this lives in
 *  its own lazy chunk rather than the composer's own module scope. */
export function ExcalidrawCanvas({ onCancel, onSave }: ExcalidrawCanvasProps) {
  const [api, setApi] = useState<ExcalidrawImperativeAPI | null>(null)
  const [saving, setSaving] = useState(false)

  const handleSave = useCallback(async () => {
    if (!api) return
    setSaving(true)
    try {
      const elements = api.getSceneElements()
      const appState = api.getAppState()
      const files = api.getFiles()
      const sceneJson = JSON.stringify({ type: 'excalidraw', version: 2, elements, files })
      const blob = await exportToBlob({ elements, appState, files, mimeType: 'image/png' })
      const pngFile = new File([blob], 'diagram.png', { type: 'image/png' })
      onSave({ sceneJson, pngFile })
    } finally {
      setSaving(false)
    }
  }, [api, onSave])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1">
        <Excalidraw excalidrawAPI={setApi} />
      </div>
      <div className="flex justify-end gap-2 p-3">
        <Button variant="ghost" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={saving || !api}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
```

Create `web/src/features/agent/composer/excalidraw-modal.tsx`:

```tsx
import { lazy, Suspense, useCallback } from 'react'
import { Dialog, DialogPopup, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import { excalidrawMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
import { toast } from '@/features/window/stores/toast-store'
import { nanoid } from 'nanoid'
import type { ExcalidrawSaveResult } from '@/features/agent/composer/excalidraw-canvas'

const ExcalidrawCanvas = lazy(() =>
  import('@/features/agent/composer/excalidraw-canvas').then((m) => ({ default: m.ExcalidrawCanvas })),
)

interface ExcalidrawModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

/**
 * Create/edit UI only — the read-only preview renderer for a settled
 * `excalidraw:{id}` fence in the transcript is Phase 2's job
 * (`excalidraw-preview.tsx`).
 *
 * ONE nanoid drives both halves of the encoding, per the design spec's
 * resolved open question ("the client mints the id and passes it through"):
 * the fence-tag id for the inline JSON, and the shortid the upload endpoint
 * folds into the PNG's filename (`{shortid}-{originalName}`). Both
 * `excalidrawMarkdown(id, ...)` and `uploadChatAttachment(..., id)` below
 * receive the SAME `id` — never let the upload mint its own.
 */
export function ExcalidrawModal({ wsId, chatId, open, onClose, onInsertMarkdown }: ExcalidrawModalProps) {
  const handleSave = useCallback(
    async ({ sceneJson, pngFile }: ExcalidrawSaveResult) => {
      const id = nanoid()
      try {
        const result = await uploadChatAttachment(wsId, chatId, { file: pngFile }, id)
        onInsertMarkdown(excalidrawMarkdown(id, sceneJson))
        onInsertMarkdown(imageMarkdown('diagram', result.ref))
        onClose()
      } catch (err) {
        toast.error("Couldn't save this drawing", err instanceof Error ? err.message : String(err))
      }
    },
    [wsId, chatId, onInsertMarkdown, onClose],
  )

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPopup className="flex h-[80vh] max-w-4xl flex-col">
        <DialogHeader>
          <DialogTitle>Excalidraw</DialogTitle>
        </DialogHeader>
        <Suspense fallback={null}>
          <ExcalidrawCanvas onCancel={onClose} onSave={handleSave} />
        </Suspense>
      </DialogPopup>
    </Dialog>
  )
}
```

Edit `web/src/features/agent/composer/agent-composer.tsx`, replacing the Task 24 placeholder:

```tsx
        {modal === 'excalidraw' && (
          <ExcalidrawModal
            wsId={props.wsId}
            chatId={props.chatId}
            open
            onClose={() => setModal(null)}
            onInsertMarkdown={(md) => editorRef.current?.insertAttachmentMarkdown(md)}
          />
        )}
```

(add `import { ExcalidrawModal } from '@/features/agent/composer/excalidraw-modal'`)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/excalidraw-modal.test.tsx`
Expected: PASS — in particular, the id-correlation assertion (`fenceMarkdown` contains `excalidraw:${uploadedId}`) is the regression test for the id-threading bug this task's own implementation must avoid. Separately: `bunx vite build` should show `excalidraw-canvas` as its own chunk, not present in the composer's/entry's chunk — mirroring the existing katex-chunk gate this codebase already runs.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/src/features/agent/composer/excalidraw-canvas.tsx web/src/features/agent/composer/excalidraw-modal.tsx web/src/features/agent/composer/agent-composer.tsx web/src/__tests__/features/agent/composer/excalidraw-modal.test.tsx
git commit -m "feat(composer): add the embedded Excalidraw drawing editor"
```

---

# Phase 4: Cross-Cutting Integration

### Task 35: Wire the Drag Handle into Every Attachment Node Component

**Files:**
- Modify: `web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx` (Task 15)
- Modify: `web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx` (Task 16)
- Modify: `web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx` (Task 17, wraps both of the above)
- Modify: `web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx` (Task 18)
- Test: `web/src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx` (extend), `web/src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx` (extend)

**Interfaces:**
- Consumes: `useAttachmentDraggable`/`AttachmentDragHandle`/`AttachmentDropLine` (Task 33)
- Produces: nothing new — this is the integration step neither Phase 2 (built before the drag-handle primitive existed) nor Phase 3 (built the primitive but doesn't own the node components) could do alone. Genuinely last: it needs both halves already merged.

**Scope note**: this wires the handle into `ChatCodeBlockElement` (covering both text-attachment and excalidraw fences, since both render through it) and `ChatAttachmentFileCard` — the three NEW node kinds this feature adds. It deliberately does **not** touch the stock image node (`MarkdownImageElement`, reused unmodified from `MarkdownImageKit` per Phase 2's Task 9/12): that component is shared with the standalone markdown file editor, and adding chat-specific drag machinery to it risks regressing an unrelated feature for a small, disclosed gap — plain image attachments don't get a drag handle in this iteration, everything else does.

- [ ] **Step 1: Write the failing test**

Extend `web/src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx`:

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
// ...existing imports...

it('renders a drag handle for a text-attachment pill', () => {
  render(
    <MarkdownMessage>{'```text-attachment:AbC123xy\nsome long pasted text\n```'}</MarkdownMessage>,
  )
  expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()
})
```

Extend `web/src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`:

```tsx
it('renders a drag handle alongside the file card', () => {
  render(
    <ChatMarkdownAssetProvider wsId="ws1">
      <MarkdownMessage>{'[report.pdf](chats/c1/attachments/report.pdf)'}</MarkdownMessage>
    </ChatMarkdownAssetProvider>,
  )
  expect(screen.getByRole('button', { name: /reorder this attachment/i })).toBeInTheDocument()
})
```

(Note: these two new assertions use `MarkdownMessage`, the *interactive* renderer, not `MarkdownMessageStatic` — a drag handle only makes sense where the block is actually editable, i.e. the composer and the interactive/streaming transcript view, not settled read-only history. `chat-composer-plugins.ts`'s `chatComposerPluginsStatic` variant should NOT register the draggable wrapper — verify `MarkdownMessageStatic`'s existing tests still pass unchanged, with no drag handle appearing there.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`
Expected: FAIL — no drag handle rendered by either component yet.

- [ ] **Step 3: Write minimal implementation**

Edit `chat-code-block-node.tsx` — wrap the whole element in the draggable primitive, only when a preview is actually showing (a plain, non-attachment code block never needs to be individually reorderable via this handle — normal block movement in the file editor's own sense is out of scope for chat per the design spec's non-goals):

```tsx
import { useComposedRef } from 'platejs/react'
import {
  AttachmentDragHandle,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
```

```tsx
export function ChatCodeBlockElement(props: PlateElementProps<TCodeBlockElement>) {
  const { element } = props
  const parsed = parseAttachmentLang(element.lang)
  const { isDragging, nodeRef, handleRef } = useAttachmentDraggable(element)

  const codeBody = (
    <pre className="overflow-x-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed [tab-size:2]">
      <code>{props.children}</code>
    </pre>
  )

  let preview: ReactNode = null
  if (parsed?.kind === 'text-attachment') {
    preview = <TextAttachmentPill text={codeBlockSource(element)} />
  } else if (parsed?.kind === 'excalidraw') {
    const scene = parseExcalidrawScene(codeBlockSource(element))
    if (scene) preview = <ExcalidrawPreview scene={scene} pngRef={findFollowingImageRef(props)} />
  }

  if (!preview) {
    return (
      <PlateElement {...props} className="my-2">
        {codeBody}
      </PlateElement>
    )
  }

  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('group/attachment relative my-2', isDragging && 'opacity-50')}
    >
      <AttachmentDragHandle dragRef={handleRef} />
      <AttachmentDropLine />
      <div contentEditable={false} className="select-none">
        {preview}
      </div>
      <div className="hidden">{codeBody}</div>
    </PlateElement>
  )
}
```

(add `import { cn } from '@/lib/utils'` if not already imported in this file)

Edit `chat-attachment-file-card.tsx`'s `ChatAttachmentFileCard` the same way — wrap with `useAttachmentDraggable(props.element)`, add the composed ref, `group/attachment` class, and render `<AttachmentDragHandle dragRef={handleRef} />` + `<AttachmentDropLine />` as the first children inside the existing `<PlateElement as="a" ...>`.

Both wrap only inside `chatComposerPlugins` (the interactive variant) — `chatComposerPluginsStatic`'s derived config already swaps out the interactive-only pieces (per its own existing derivation pattern, `chat-composer-plugins.ts:127-141`); confirm `ChatCodeBlockElement`/`ChatLinkElement` are NOT among the components that static variant overrides away, since the static-vs-interactive distinction here needs to be "does the SAME component render a drag handle" — add a `static?: boolean` prop threaded from `chatComposerPluginsStatic`'s own plugin configuration if the existing derivation mechanism doesn't already give a clean way to suppress it, checking the real derivation code before deciding which approach fits its existing shape.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bunx vitest run src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx`
Expected: PASS. Also re-run both files' full suites (not just the new tests) plus `markdown-message-static.test.tsx` to confirm the static/read-only path renders no drag handle.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx web/src/__tests__/features/agent/composer/plate/attachments/chat-code-block-node.test.tsx web/src/__tests__/features/agent/composer/plate/attachments/chat-attachment-file-card.test.tsx
git commit -m "feat(chat): wire the drag-to-reorder handle into attachment node components"
```

---

## Deferred to a follow-up (accepted gaps, per the design spec)

- **Orphaned files from discarded drafts**: attachments upload eagerly on attach, before send. Removing the attachment from a draft, or discarding the draft, leaves the uploaded file on disk with nothing referencing it — no GC in this plan, matching the design spec's own explicit v1 scope trim.
- **Live spike of provider-CLI read-outside-cwd behavior**: moot for this plan's chosen delivery mechanism (materialize-into-worktree), since the CLI never needs to read outside its own cwd at all — noted here only so a future session doesn't reintroduce direct durable-path references without re-deriving why that was rejected.
- **Image attachments don't get a drag handle** (Task 35's scope note) — only text-attachment, excalidraw, and file-card do, to avoid touching the shared `MarkdownImageElement`.

---

## Self-Review Notes

Ran the required spec-coverage, placeholder, and type-consistency passes over this plan before finalizing:

- **Spec coverage**: every attachment kind (text, image, CSV, file, Excalidraw), every threshold rule, the two-gate CSV rule, id-suffixed fence tags, the materialize-at-dispatch delivery mechanism and its dual cleanup, the new rendering path, and every open question the spec deferred to this stage (thresholds, upload contract, Excalidraw library, drag-handle scope, CSV parser, id-scheme, plus-button placement) all have a task. The one spec item NOT implemented here — orphaned-file GC — was explicitly marked out-of-scope-for-v1 in the spec itself, not missed.
- **Placeholder scan**: no "TODO"/"TBD"/"add appropriate X" phrasing in any Step 3 implementation. The one remaining soft spot — Task 35's static/interactive drag-handle suppression — is flagged as "check the real derivation code before deciding" rather than asserting an unverified specific mechanism, which is a judgment call for whoever implements it to resolve against the actual current `chat-composer-plugins.ts`, not a placeholder for missing design work.
- **Type consistency, fixed inline during assembly** (this is the actual value the assembly pass added over the three independently-drafted phases):
  - `uploadChatAttachment`'s signature didn't exist anywhere until Task 20 was added — both Phase 2 and Phase 3 tasks were drafted assuming it, with no one actually building it. Fixed by inserting Task 20 first in Phase 3.
  - Backend's response field `fileName` (JSON) vs. every frontend call site's `result.filename` — Task 20 does the mapping once, centrally, so no other task needs to know the backend's exact casing.
  - Backend requires a client-supplied `id` on upload; the originally-drafted `uploadChatAttachment(wsId, chatId, input)` signature had no way to pass one, which would have silently broken Excalidraw's id-correlation requirement. Fixed by making `id` an optional 4th parameter, and explicitly fixing Task 34's `handleSave` (which, before this fix, generated a `nanoid()` for the fence tag but never passed it to the upload call) to pass it through — Task 34's own test now asserts this correlation directly, as a regression guard.
  - `PencilIcon` was imported by Task 23 (`composer-plus-button.tsx`) but never defined by any task — fixed by adding it to Task 22 alongside `PlusIcon`/`FileIcon`.
  - The drag-handle primitive (Task 33) and the node components that need to render it (Phase 2's Tasks 15/16/17/18) were built by two independently-drafted phases with no task ever connecting them — fixed by adding Task 35.
  - Backend's `SyntheticName`/`ContentType` utilities (Task 2) were written and tested but never called from the actual upload handler (Task 4's original draft required a non-blank filename with no fallback) — fixed by wiring them into `readAttachmentFromMultipart` as a defensive backstop.
