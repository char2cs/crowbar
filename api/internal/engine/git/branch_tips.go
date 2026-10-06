package git

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// racyWindow is how recently a ref may have been written and still be distrusted:
// a coarse filesystem clock can give two writes one timestamp, and a ref's size
// never changes, so the same rule git applies to its index applies here.
const racyWindow = 2 * time.Second

// tipMemoryCap bounds how many clones' tips are remembered.
const tipMemoryCap = 64

// tipMemory remembers each clone's branch tips under a stamp of its ref store,
// so a read is served from memory exactly while the ref store is unchanged. The
// ref store itself is the owner: the stamp is compared on every read, nothing
// expires and nothing has to be told that a branch moved.
type tipMemory struct {
	mu sync.Mutex
	m  map[string]stampedTips
}

type stampedTips struct {
	stamp string
	tips  map[string]string
}

// branchTips is every local branch's commit, by short name; empty when the read
// fails, which sends every pair down the by-name path. The caller holds the
// repo's read lock.
func (e *engine) branchTips(
	ctx context.Context,
	repoPath string,
) map[string]string {
	common := e.resolveCommonDir(ctx, repoPath)
	stamp, trusted := refStoreStamp(common)
	if trusted {
		if tips, ok := e.tips.get(common, stamp); ok {
			return tips
		}
	}
	tips, ok := e.readBranchTips(ctx, repoPath)
	if trusted && ok {
		e.tips.put(common, stamp, tips)
	}
	return tips
}

func (e *engine) readBranchTips(
	ctx context.Context,
	repoPath string,
) (map[string]string, bool) {
	r := e.exec(ctx, repoPath, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads")
	tips := map[string]string{}
	if r.ExitCode != 0 {
		return tips, false
	}
	for line := range strings.SplitSeq(r.Stdout, "\n") {
		name, sha, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok {
			tips[name] = sha
		}
	}
	return tips, true
}

func (m *tipMemory) get(common, stamp string) (map[string]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.m[common]; ok && s.stamp == stamp {
		return s.tips, true
	}
	return nil, false
}

func (m *tipMemory) put(common, stamp string, tips map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil || len(m.m) >= tipMemoryCap {
		m.m = make(map[string]stampedTips, tipMemoryCap)
	}
	m.m[common] = stampedTips{stamp: stamp, tips: tips}
}

// refStoreStamp fingerprints a clone's loose refs and packed-refs by name, size
// and modification time. It reports untrusted for a layout it cannot read this
// way (reftable), for a ref written inside racyWindow, and on any read error.
func refStoreStamp(common string) (string, bool) {
	if _, err := os.Stat(filepath.Join(common, "reftable")); err == nil {
		return "", false
	}
	var b strings.Builder
	now := time.Now()
	add := func(name string, info fs.FileInfo) bool {
		if now.Sub(info.ModTime()) < racyWindow {
			return false
		}
		b.WriteString(name)
		b.WriteByte(' ')
		b.WriteString(strconv.FormatInt(info.Size(), 10))
		b.WriteByte(' ')
		b.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
		b.WriteByte('\n')
		return true
	}
	if info, err := os.Stat(filepath.Join(common, "packed-refs")); err == nil && !add("packed-refs", info) {
		return "", false
	}
	trusted := true
	root := filepath.Join(common, "refs", "heads")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		trusted = add(p, info)
		if !trusted {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil || !trusted {
		return "", false
	}
	return b.String(), true
}
