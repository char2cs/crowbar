package defaultbranch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// runner returns a RefRunner that maps each expected git args call to a result.
func runner(
	responses map[string]string,
) RefRunner {
	return func(args ...string) (string, bool) {
		key := args[0]
		if len(args) > 1 {
			key = args[len(args)-1]
		}
		v, ok := responses[key]
		return v, ok
	}
}

func TestResolve_SymbolicRefOriginHead(t *testing.T) {
	r := runner(map[string]string{
		"symbolic-ref": "refs/remotes/origin/HEAD",
	})
	got := Resolve(r, []string{"main", "develop", "master"})
	assert.Equal(t, "HEAD", got)
}

func TestResolve_FirstConfigBranchConfirmedByRevParse(t *testing.T) {
	r := func(args ...string) (string, bool) {
		// symbolic-ref fails
		if args[0] == "symbolic-ref" {
			return "", false
		}
		// rev-parse succeeds for "main"
		if args[len(args)-1] == "main" {
			return "abc123", true
		}
		return "", false
	}
	got := Resolve(r, []string{"main", "develop", "master"})
	assert.Equal(t, "main", got)
}

func TestResolve_SecondConfigBranchWhenFirstAbsent(t *testing.T) {
	r := func(args ...string) (string, bool) {
		if args[0] == "symbolic-ref" {
			return "", false
		}
		if args[len(args)-1] == "develop" {
			return "def456", true
		}
		return "", false
	}
	got := Resolve(r, []string{"main", "develop", "master"})
	assert.Equal(t, "develop", got)
}

func TestResolve_FallsBackToCurrentHead(t *testing.T) {
	r := func(args ...string) (string, bool) {
		if args[0] == "symbolic-ref" && args[len(args)-1] == "HEAD" {
			return "refs/heads/feature-x", true
		}
		// symbolic-ref for origin/HEAD fails
		if args[0] == "symbolic-ref" {
			return "", false
		}
		// rev-parse for config branches all fail
		return "", false
	}
	got := Resolve(r, []string{"main", "master"})
	assert.Equal(t, "feature-x", got)
}

func TestResolve_NeverEmpty_AllFail(t *testing.T) {
	r := func(_ ...string) (string, bool) { return "", false }
	got := Resolve(r, []string{"main"})
	assert.NotEmpty(t, got)
	assert.Equal(t, "main", got)
}

func TestResolve_EmptyConfigList_FallsBackToHead(t *testing.T) {
	r := func(args ...string) (string, bool) {
		if args[0] == "symbolic-ref" && args[len(args)-1] == "HEAD" {
			return "refs/heads/my-branch", true
		}
		return "", false
	}
	got := Resolve(r, []string{})
	assert.Equal(t, "my-branch", got)
}

func TestResolve_EmptyConfigListAllFail_ReturnsMain(t *testing.T) {
	r := func(_ ...string) (string, bool) { return "", false }
	got := Resolve(r, []string{})
	assert.Equal(t, "main", got)
}
