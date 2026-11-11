package attachments

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// stagingDir holds in-flight chunked uploads beside the finished attachments; a stored name never
// starts with ".", so the two cannot collide.
const stagingDir = ".uploads"

// stagingTTL is how long an unfinished upload may sit before the next BeginUpload in the same chat
// deletes it: an abandoned upload is reaped without a timer.
const stagingTTL = 24 * time.Hour

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
	if sanitizeName(originalName) == "" {
		return 0, fmt.Errorf("attachments: original name required: %w", problem.ErrInvalidArgument)
	}
	if size < 0 || size > MaxBytes {
		return 0, fmt.Errorf("attachments: %w", safepath.ErrFileTooLarge)
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
	return sizeAndClose(f)
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

func sizeAndClose(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, errors.Join(fmt.Errorf("attachments: stat staging file: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("attachments: close staging file: %w", err)
	}
	return info.Size(), nil
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
	//nolint:gosec // G304: part is built from a validated id inside the chat-scoped dir.
	f, err := os.OpenFile(part, os.O_RDWR, 0o600)
	if err != nil {
		return 0, ErrNotFound
	}
	have, err := fileSize(f)
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

func fileSize(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("attachments: stat staging file: %w", err)
	}
	return info.Size(), nil
}

// CompleteUpload turns the staging file into the stored attachment "<id>-<originalName>" once it
// holds exactly size bytes. An existing attachment is problem.ErrConflict, never overwritten.
func CompleteUpload(dir, id, originalName string, size int64) (fileName, contentType string, err error) {
	part, err := stagingPath(dir, id)
	if err != nil {
		return "", "", err
	}
	base := sanitizeName(originalName)
	if base == "" {
		return "", "", fmt.Errorf("attachments: original name required: %w", problem.ErrInvalidArgument)
	}
	//nolint:gosec // G304: part is built from a validated id inside the chat-scoped dir.
	f, err := os.Open(part)
	if err != nil {
		return "", "", ErrNotFound
	}
	head := make([]byte, 512)
	n, readErr := io.ReadFull(f, head)
	have, sizeErr := fileSize(f)
	if err := errors.Join(sizeErr, f.Close()); err != nil {
		return "", "", err
	}
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		return "", "", fmt.Errorf("attachments: read staging file: %w", readErr)
	}
	if have != size {
		return "", "", fmt.Errorf("attachments: holding %d of %d bytes: %w", have, size, problem.ErrConflict)
	}
	fileName = id + "-" + base
	if err := os.Link(part, filepath.Join(dir, fileName)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", "", fmt.Errorf("attachments: %q already exists: %w", fileName, problem.ErrConflict)
		}
		return "", "", fmt.Errorf("attachments: publish upload: %w", err)
	}
	if err := os.Remove(part); err != nil {
		return "", "", fmt.Errorf("attachments: remove staging file: %w", err)
	}
	return fileName, ContentType(head[:n]), nil
}

// AbortUpload deletes the staging file; aborting an upload that is already gone succeeds.
func AbortUpload(dir, id string) error {
	part, err := stagingPath(dir, id)
	if err != nil {
		return err
	}
	if err := os.Remove(part); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("attachments: abort upload: %w", err)
	}
	return nil
}
