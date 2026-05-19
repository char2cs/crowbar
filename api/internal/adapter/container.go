package adapter

import (
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

type Container struct {
	Store *sqlite.Store
}

func New() (*Container, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".crowbar")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	store, err := sqlite.New(filepath.Join(dir, "crowbar.db"))
	if err != nil {
		return nil, err
	}

	return &Container{Store: store}, nil
}

func (c *Container) Close() {
	_ = c.Store.Close()
}
