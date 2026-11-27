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

const racyWindow = 2 * time.Second

const tipMemoryCap = 64

type tipMemory struct {
	mu sync.Mutex
	m  map[string]stampedTips
}

type stampedTips struct {
	stamp string
	tips  map[string]string
}

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
