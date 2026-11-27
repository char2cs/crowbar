package content

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

const maxReadBytes = 25 << 20

var ErrFileTooLarge = safepath.ErrFileTooLarge

func Read(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	return readWithCap(repoPath, filePath, maxReadBytes)
}

func ReadAbs(
	abs string,
) (domain.FileContent, error) {
	if err := requireAbs(abs); err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", abs, err)
	}
	fc, err := readFile(abs, maxReadBytes)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", abs, err)
	}
	return fc, nil
}

func readWithCap(
	repoPath string,
	filePath string,
	cap int64,
) (domain.FileContent, error) {
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	fc, err := readFile(full, cap)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("content: read %s: %w", filePath, err)
	}
	return fc, nil
}

func readFile(
	full string,
	cap int64,
) (domain.FileContent, error) {
	info, err := os.Stat(full)
	if err != nil {
		return domain.FileContent{}, err
	}
	if info.Size() > cap {
		return domain.FileContent{}, ErrFileTooLarge
	}

	f, err := os.Open(full) //nolint:gosec // G304: full is safepath-resolved or an absolute path the file usecase resolved
	if err != nil {
		return domain.FileContent{}, err
	}
	defer f.Close() //nolint:errcheck // read-only handle: a close error cannot lose data

	data, err := io.ReadAll(io.LimitReader(f, cap+1))
	if err != nil {
		return domain.FileContent{}, err
	}
	if int64(len(data)) > cap {
		return domain.FileContent{}, ErrFileTooLarge
	}

	if isBinary(data) {
		return domain.FileContent{
			Content:  base64.StdEncoding.EncodeToString(data),
			Encoding: "base64",
		}, nil
	}

	return domain.FileContent{Content: string(data)}, nil
}

func Write(
	repoPath string,
	filePath string,
	content string,
	encoding string,
) error {
	data, err := decodeContent(content, encoding)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	if err := writeFile(full, data); err != nil {
		return fmt.Errorf("content: write %s: %w", filePath, err)
	}
	return nil
}

func WriteAbs(
	abs string,
	content string,
	encoding string,
) error {
	if err := requireAbs(abs); err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	data, err := decodeContent(content, encoding)
	if err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	if err := writeFile(abs, data); err != nil {
		return fmt.Errorf("content: write %s: %w", abs, err)
	}
	return nil
}

func writeFile(
	full string,
	data []byte,
) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("mkdirall: %w", err)
	}
	return writeAtomic(full, data)
}

func requireAbs(
	abs string,
) error {
	if !filepath.IsAbs(abs) || filepath.Clean(abs) != abs {
		return safepath.ErrNotAbsolute
	}
	return nil
}

func writeAtomic(
	full string,
	data []byte,
) (err error) {
	if fi, err := os.Lstat(full); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return writeThrough(full, data)
	}

	perm := os.FileMode(0o600)
	if info, err := os.Stat(full); err == nil {
		perm = info.Mode().Perm()
		if info.Mode().IsRegular() {
			if err := requireWritable(full); err != nil {
				return err
			}
		}
	}

	dir := filepath.Dir(full)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(full)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpName))
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, full)
}

func requireWritable(full string) error {
	f, err := os.OpenFile(full, os.O_WRONLY, 0) //nolint:gosec // G304: full is safepath-resolved or an absolute path the file usecase resolved
	if err != nil {
		return err
	}
	return f.Close()
}

func writeThrough(
	link string,
	data []byte,
) error {
	f, err := os.OpenFile(link, os.O_WRONLY|os.O_TRUNC, 0) //nolint:gosec // G304: link is safepath-resolved or an absolute path the file usecase resolved
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", safepath.ErrDanglingSymlink, link)
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

func decodeContent(
	content string,
	encoding string,
) ([]byte, error) {
	switch encoding {
	case "", "utf8":
		return []byte(content), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("decode base64: %w", err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported encoding %q", encoding)
	}
}

func isBinary(
	data []byte,
) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(data)
}
