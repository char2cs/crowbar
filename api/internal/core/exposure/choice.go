package exposure

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const choiceFile = "network.json"

type choice struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
}

func load(root string) (choice, error) {
	raw, err := os.ReadFile(filepath.Join(root, choiceFile)) //nolint:gosec // G304: a fixed name under the daemon's own home
	if errors.Is(err, fs.ErrNotExist) {
		return choice{Address: DefaultAddress}, nil
	}
	if err != nil {
		return choice{}, fmt.Errorf("exposure: read %s: %w", choiceFile, err)
	}
	var c choice
	if err := json.Unmarshal(raw, &c); err != nil {
		return choice{}, fmt.Errorf("exposure: %s is not valid: %w", choiceFile, err)
	}
	if c.Address == "" {
		c.Address = DefaultAddress
	}
	return c, nil
}

func save(root string, c choice) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("exposure: encode: %w", err)
	}
	path := filepath.Join(root, choiceFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("exposure: write %s: %w", choiceFile, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("exposure: replace %s: %w", choiceFile, err)
	}
	return nil
}

func validate(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("exposure: %q is not a host:port address: %w", address, err)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("exposure: the port in %q must be a number from 1 to 65535", address)
	}
	if !isExposed(host) {
		return fmt.Errorf("exposure: %q binds loopback only; choose an address other devices can reach", address)
	}
	return nil
}
