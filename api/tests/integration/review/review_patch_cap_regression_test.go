//go:build integration

package review_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func (s *ReviewSuite) TestRegression_CappedPatchOfOneHugeHunkCarriesContent() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)

	var body strings.Builder
	for i := 1; i <= 400; i++ {
		fmt.Fprintf(&body, "generated line %d\n", i)
	}
	kit.CommitFile(s.T(), worktree, "src/huge.ts", body.String(), "add a file far larger than the cap")

	const cap = 50
	resp := s.env.GET(s.T(), s.base()+"/review/patch?path=src/huge.ts&maxLines="+fmt.Sprint(cap))
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	patch := string(raw)

	s.Require().Contains(patch, "diff --git", "the patch must carry its file header")

	s.Assert().Equal("true", resp.Header.Get("X-Crowbar-Diff-Truncated"),
		"a 400-line file under a 50-line cap is truncated, and the client needs to know")

	s.Require().Contains(patch, "@@ ",
		"the file's only hunk is larger than the cap; dropping it renders the file blank")
	s.Assert().Contains(patch, "+generated line 1\n",
		"the truncated hunk must carry real content, not just a header")

	lines := strings.Count(patch, "\n")
	s.Assert().LessOrEqual(lines, cap, "the cut must never run past the cap")
	s.Assert().Greater(lines, cap/2,
		"the cap must be filled, not abandoned at the file header")

	s.assertHunkCountsMatchBody(patch)
}

func (s *ReviewSuite) assertHunkCountsMatchBody(patch string) {
	s.T().Helper()

	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "@@ ") {
			continue
		}
		var oldStart, oldCount, newStart, newCount int
		_, err := fmt.Sscanf(lines[i], "@@ -%d,%d +%d,%d @@", &oldStart, &oldCount, &newStart, &newCount)
		s.Require().NoErrorf(err, "unparseable hunk header %q", lines[i])

		gotOld, gotNew := 0, 0
		for j := i + 1; j < len(lines); j++ {
			line := lines[j]
			if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "diff --git ") {
				break
			}
			switch {
			case strings.HasPrefix(line, "+"):
				gotNew++
			case strings.HasPrefix(line, "-"):
				gotOld++
			case strings.HasPrefix(line, `\`):
			default:
				gotOld++
				gotNew++
			}
		}
		s.Assert().Equalf(oldCount, gotOld, "hunk %q: old-side count disagrees with the body", lines[i])
		s.Assert().Equalf(newCount, gotNew, "hunk %q: new-side count disagrees with the body", lines[i])
	}
}
