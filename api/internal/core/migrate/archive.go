package migrate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// backupExt names a backup: one gzip tar that a restore unpacks over the home.
const backupExt = ".tar.gz"

// digests are the SHA-256 of each regular file a backup holds, by the
// slash-separated path the file has under the home.
type digests map[string][sha256.Size]byte

// archiver writes a backup's entries, hashing each file as it is copied. bytes
// counts the trees it took, the data a backup is measured by.
type archiver struct {
	tw    *tar.Writer
	sums  digests
	bytes int64
}

func newArchiver(
	w io.Writer,
) *archiver {
	return &archiver{tw: tar.NewWriter(w), sums: digests{}}
}

func (a *archiver) addDir(
	rel string,
	info fs.FileInfo,
) error {
	return a.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir, Name: rel + "/", Mode: 0o700, ModTime: info.ModTime(),
	})
}

// addBytes writes data as the file rel, for a value the home holds in no file
// of its own (the layout of a home without a LAYOUT file).
func (a *archiver) addBytes(
	rel string,
	data []byte,
) error {
	if err := a.tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: rel, Mode: 0o600, Size: int64(len(data))}); err != nil {
		return err
	}
	if _, err := a.tw.Write(data); err != nil {
		return err
	}
	a.sums[rel] = sha256.Sum256(data)
	return nil
}

func (a *archiver) addFile(
	rel string,
	src string,
	info fs.FileInfo,
) (err error) {
	in, err := os.Open(src) //nolint:gosec // G304: src comes from walking the home's own tree
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	header := &tar.Header{Typeflag: tar.TypeReg, Name: rel, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime()}
	if err := a.tw.WriteHeader(header); err != nil {
		return err
	}
	hash := sha256.New()
	// CopyN holds the entry to the size its header promised: the home is locked
	// and no store is open, so a file that changes size here is a fault.
	if _, err := io.CopyN(io.MultiWriter(a.tw, hash), in, info.Size()); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	a.sums[rel] = [sha256.Size]byte(hash.Sum(nil))
	return nil
}

// addTree writes everything under dir as entries under prefix. Only regular
// files and directories are legal; skip leaves out an entry that is not data.
func (a *archiver) addTree(
	dir string,
	prefix string,
	skip func(root, path string, entry fs.DirEntry) bool,
) error {
	root, found, err := resolveRoot(dir)
	if err != nil || !found {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skip(root, path, entry) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		info, err := entry.Info()
		switch {
		case err != nil:
			return err
		case entry.IsDir():
			return a.addDir(name, info)
		case entry.Type().IsRegular():
			a.bytes += info.Size()
			return a.addFile(name, path, info)
		default:
			return unsupportedEntry(path, entry.Type())
		}
	})
}

// close flushes the tar stream; the caller closes what it wrote into.
func (a *archiver) close() error {
	return a.tw.Close()
}

// writeArchive writes fill's entries to a new gzip tar at path, flushes it, and
// returns what it holds.
func writeArchive(
	path string,
	fill func(*archiver) error,
) (*archiver, error) {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: path is inside the backups directory this run owns
	if err != nil {
		return nil, err
	}
	zw := gzip.NewWriter(out)
	a := newArchiver(zw)
	err = fill(a)
	if err == nil {
		err = a.close()
	}
	if err == nil {
		err = zw.Close()
	}
	if err == nil {
		err = out.Sync()
	}
	return a, errors.Join(err, out.Close())
}

// readDigests reads the archive at path back and hashes every file it holds.
// An entry that is not a plain relative path is an error: a restore must not
// write outside the home.
func readDigests(
	path string,
) (_ digests, err error) {
	in, err := os.Open(path) //nolint:gosec // G304: path is a backup in the home's own backups directory
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	zr, err := gzip.NewReader(in)
	if err != nil {
		return nil, fmt.Errorf("migrate: backup: read %s: %w", path, err)
	}
	out := digests{}
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("migrate: backup: read %s: %w", path, err)
		}
		if !filepath.IsLocal(filepath.FromSlash(strings.TrimSuffix(header.Name, "/"))) {
			return nil, fmt.Errorf("migrate: backup: %s holds %q, which is not under the home", path, header.Name)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, tr); err != nil { //nolint:gosec // G110: the archive is one this daemon wrote and verified
			return nil, fmt.Errorf("migrate: backup: read %s: %w", path, err)
		}
		out[header.Name] = [sha256.Size]byte(hash.Sum(nil))
	}
}

// verifyArchive reads the archive at path back and requires it to hold exactly
// the files, byte for byte, that were written into it.
func verifyArchive(
	path string,
	want digests,
) error {
	got, err := readDigests(path)
	if err != nil {
		return err
	}
	if !maps.Equal(got, want) {
		return fmt.Errorf("migrate: backup: %s does not read back as written", path)
	}
	return nil
}

// SameInBackup reports which of rels, slash-separated paths under the home at
// root, the backup at path holds byte for byte as they are now. A directory
// backup, as builds before archives wrote them, holds state/ alone.
func SameInBackup(
	path string,
	root string,
	rels []string,
) (map[string]bool, error) {
	held, err := backupDigests(path, rels)
	if err != nil {
		return nil, err
	}
	same := map[string]bool{}
	for _, rel := range rels {
		want, ok := held[rel]
		if !ok {
			continue
		}
		live, err := fileDigest(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		same[rel] = live == want
	}
	return same, nil
}

func backupDigests(
	path string,
	rels []string,
) (digests, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("migrate: backup: inspect %s: %w", path, err)
	}
	if !info.IsDir() {
		return readDigests(path)
	}
	held := digests{}
	for _, rel := range rels {
		if !strings.HasPrefix(rel, "state/") {
			continue
		}
		sum, err := fileDigest(filepath.Join(path, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		held[rel] = sum
	}
	return held, nil
}

func fileDigest(
	path string,
) (_ [sha256.Size]byte, err error) {
	in, err := os.Open(path) //nolint:gosec // G304: path is under the home or one of its backups
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	hash := sha256.New()
	if _, err := io.Copy(hash, in); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("migrate: hash %s: %w", path, err)
	}
	return [sha256.Size]byte(hash.Sum(nil)), nil
}
