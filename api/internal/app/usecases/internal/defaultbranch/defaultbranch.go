// Package defaultbranch resolves the default branch of a local git repository
// without executing real git commands — callers inject a RefRunner (00 §5.2).
package defaultbranch

import "strings"

// RefRunner executes a git sub-command and returns its trimmed output.
// Returns (output, true) on success; ("", false) on any failure.
type RefRunner func(args ...string) (string, bool)

// Resolve returns the default branch: origin/HEAD, else the first configList entry
// rev-parse verifies, else the current HEAD, else the first configList entry or "main".
func Resolve(
	runner RefRunner,
	configList []string,
) string {
	if b := fromOriginHead(runner); b != "" {
		return b
	}
	if b := fromConfigList(runner, configList); b != "" {
		return b
	}
	if b := fromCurrentHead(runner); b != "" {
		return b
	}
	return neverEmptyFallback(configList)
}

func fromOriginHead(
	runner RefRunner,
) string {
	ref, ok := runner("symbolic-ref")
	if !ok || ref == "" {
		return ""
	}
	return lastPathSegment(ref)
}

func fromConfigList(
	runner RefRunner,
	configList []string,
) string {
	for _, branch := range configList {
		_, ok := runner("rev-parse", "--verify", branch)
		if ok {
			return branch
		}
	}
	return ""
}

func fromCurrentHead(
	runner RefRunner,
) string {
	ref, ok := runner("symbolic-ref", "HEAD")
	if !ok || ref == "" {
		return ""
	}
	return lastPathSegment(ref)
}

func lastPathSegment(
	ref string,
) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func neverEmptyFallback(
	configList []string,
) string {
	if len(configList) > 0 {
		return configList[0]
	}
	return "main"
}
