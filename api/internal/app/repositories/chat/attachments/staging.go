package attachments

import (
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// stagingDir holds in-flight chunked uploads beside the finished attachments; a stored name never
// starts with ".", so the two cannot collide. An upload unfinished for stagingTTL is reaped by the
// next BeginUpload in the same chat, without a timer.
const (
	stagingDir = ".uploads"
	stagingTTL = 24 * time.Hour
)

// stagingLocks serialise the check-then-write of one staging file: two overlapping chunks (a retry
// racing its slow original) must not interleave. Striped by path so unrelated uploads never wait.
var (
	stagingLocks [64]sync.Mutex
	stagingSeed  = maphash.MakeSeed()
)

func lockStaging(part string) (unlock func()) {
	mu := &stagingLocks[maphash.String(stagingSeed, part)%uint64(len(stagingLocks))]
	mu.Lock()
	return mu.Unlock
}

func stagingPath(dir, id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("attachments: invalid id: %w", problem.ErrInvalidArgument)
	}
	return filepath.Join(dir, stagingDir, id+".part"), nil
}

// BeginUpload opens (or reopens) the staging file for a chunked upload of originalName and answers
// how many bytes it already holds, so an interrupted upload resumes there. The name and size are
// checked now so a doomed upload fails before any byte is sent.
func BeginUpload(dir, id, originalName string, size int64) (received int64, err error) {
	part, err := stagingPath(dir, id)
	if err != nil {
		return 0, err
	}
	base := sanitizeName(originalName)
	if base == "" {
		return 0, fmt.Errorf("attachments: original name required: %w", problem.ErrInvalidArgument)
	}
	if size < 0 || size > MaxBytes {
		return 0, fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
	}
	defer lockStaging(part)()
	if held, _, found, err := storedUpload(dir, id+"-"+base, size); err != nil || found {
		return held, err
	}
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		return 0, fmt.Errorf("attachments: create staging directory: %w", err)
	}
	sweepStaging(filepath.Dir(part))
	//nolint:gosec // G304: part is built from a validated id inside the chat-scoped dir.
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return 0, fmt.Errorf("attachments: open staging file: %w", err)
	}
	_, held, err := sniff(f, 0)
	return held, errors.Join(err, f.Close())
}

func sweepStaging(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) <= stagingTTL {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			slog.Warn("attachments: sweep stale upload", "file", e.Name(), "err", err)
		}
	}
}

// AppendUpload writes r at offset, which must be the number of bytes already held; a chunk wholly
// held already is a no-op, so a resend after a lost reply is safe. It answers the bytes held.
func AppendUpload(dir, id string, offset int64, r io.Reader) (received int64, err error) {
	part, err := stagingPath(dir, id)
	if err != nil {
		return 0, err
	}
	chunk, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("attachments: read chunk: %w", err)
	}
	if offset < 0 || offset+int64(len(chunk)) > MaxBytes {
		return 0, fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
	}
	defer lockStaging(part)()
	//nolint:gosec // G304: part is built from a validated id inside the chat-scoped dir.
	f, err := os.OpenFile(part, os.O_RDWR, 0o600)
	if err != nil {
		return 0, ErrNotFound
	}
	_, have, err := sniff(f, 0)
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}
	end := offset + int64(len(chunk))
	if offset > have || (offset < have && end > have) {
		return 0, errors.Join(fmt.Errorf("attachments: chunk at %d, holding %d: %w", offset, have, problem.ErrConflict), f.Close())
	}
	if end <= have {
		return have, f.Close()
	}
	if _, err := f.WriteAt(chunk, offset); err != nil {
		return 0, errors.Join(fmt.Errorf("attachments: write chunk: %w", err), f.Truncate(have), f.Close())
	}
	return end, f.Close()
}

// CompleteUpload turns the staging file into the stored attachment "<id>-<originalName>" once it
// holds exactly size bytes. An existing attachment is problem.ErrConflict, never overwritten, except
// one this very upload already stored: that is a retry whose reply was lost, and it succeeds again.
func CompleteUpload(dir, id, originalName string, size int64) (fileName, contentType string, err error) {
	part, err := stagingPath(dir, id)
	if err != nil {
		return "", "", err
	}
	base := sanitizeName(originalName)
	if base == "" {
		return "", "", fmt.Errorf("attachments: original name required: %w", problem.ErrInvalidArgument)
	}
	defer lockStaging(part)()
	fileName = id + "-" + base
	//nolint:gosec // G304: part is built from a validated id inside the chat-scoped dir.
	f, err := os.OpenFile(part, os.O_RDWR, 0o600)
	if err != nil {
		_, contentType, found, err := storedUpload(dir, fileName, size)
		if err == nil && !found {
			err = ErrNotFound
		}
		return fileName, contentType, err
	}
	head, have, err := sniff(f, size)
	if err := errors.Join(err, f.Close()); err != nil {
		return "", "", err
	}
	if have != size {
		return "", "", fmt.Errorf("attachments: holding %d of %d bytes: %w", have, size, problem.ErrConflict)
	}
	if err := os.Link(part, filepath.Join(dir, fileName)); err != nil {
		if errors.Is(err, os.ErrExist) {
			err = fmt.Errorf("attachments: %q already exists: %w", fileName, problem.ErrConflict)
		}
		return "", "", fmt.Errorf("attachments: publish upload: %w", err)
	}
	if err := os.Remove(part); err != nil {
		return "", "", fmt.Errorf("attachments: remove staging file: %w", err)
	}
	return fileName, ContentType(head), nil
}

// sniff is a file's size and the head ContentType judges it by; a file of exactly syncAt bytes is
// also flushed, so a crash after it is published cannot leave a zero-filled attachment.
func sniff(f *os.File, syncAt int64) (head []byte, have int64, err error) {
	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("attachments: stat file: %w", err)
	}
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, 0, fmt.Errorf("attachments: read file: %w", err)
	}
	if syncAt > 0 && info.Size() == syncAt {
		err = f.Sync()
	} else {
		err = nil
	}
	return buf[:n], info.Size(), err
}

// AbortUpload deletes the staging file; aborting an upload that is already gone succeeds.
func AbortUpload(dir, id string) error {
	part, err := stagingPath(dir, id)
	if err != nil {
		return err
	}
	defer lockStaging(part)()
	if err := os.Remove(part); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attachments: abort upload: %w", err)
	}
	return nil
}

// storedUpload recognises an upload that already completed: Complete's reply can be lost after the
// attachment is published, and the client's retry must then see it done, not fail or re-send it.
func storedUpload(dir, fileName string, size int64) (held int64, contentType string, found bool, err error) {
	//nolint:gosec // G304: fileName is the validated id plus a sanitised base inside the chat-scoped dir.
	f, err := os.Open(filepath.Join(dir, fileName))
	if errors.Is(err, os.ErrNotExist) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("attachments: open stored attachment: %w", err)
	}
	head, have, err := sniff(f, 0)
	if err := errors.Join(err, f.Close()); err != nil {
		return 0, "", false, err
	}
	if have != size {
		return 0, "", false, fmt.Errorf("attachments: %q already exists: %w", fileName, problem.ErrConflict)
	}
	return have, ContentType(head), true, nil
}
