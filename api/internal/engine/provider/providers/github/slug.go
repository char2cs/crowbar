package github

import (
	"fmt"
	"strings"
)

// slugFromURL parses HTTPS and SSH remote URLs into "owner/repo".
func slugFromURL(
	url string,
) (string, error) {
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}

	// HTTPS: https://github.com/owner/repo
	if idx := strings.Index(url, "://"); idx >= 0 {
		path := url[idx+3:]
		slash := strings.Index(path, "/")
		if slash >= 0 {
			return path[slash+1:], nil
		}
	}

	return "", fmt.Errorf("unrecognised remote URL format: %q", url)
}
