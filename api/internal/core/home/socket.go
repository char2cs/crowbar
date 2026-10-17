package home

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
)

const socketFile = "crowbar.sock"

// Socket is where the daemon for this home listens. The default home binds
// inside itself; an overridden home binds a short, hashed name in the temp
// dir because macOS caps sun_path at 104 bytes and override homes exceed it.
func (h Home) Socket() string {
	if !h.overridden {
		return filepath.Join(h.root, socketFile)
	}
	return filepath.Join(os.TempDir(), socketName(h.root))
}

func socketName(
	root string,
) string {
	// gosec G104 exempts (hash.Hash).Write, but not the Hash64 fnv returns.
	var digest hash.Hash = fnv.New64a()
	digest.Write([]byte(root))
	return fmt.Sprintf("crowbar-%x.sock", binary.BigEndian.Uint64(digest.Sum(nil)))
}
