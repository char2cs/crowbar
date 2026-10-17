package migrate

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func render(
	tb testing.TB,
	r Report,
) []string {
	tb.Helper()
	var out bytes.Buffer
	require.NoError(tb, r.Render(&out))
	return strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
}

func pendingReport() Report {
	return Report{
		Home: "/home/me/.crowbar", Layout: 0, Known: 1,
		Steps: []StepPlan{
			{
				Step: Step{Number: 2, Name: "workspace-paths-table", Phase: PreOpen, From: 0, To: 0},
				Plan: Plan{
					Actions:       []Action{{Kind: Convert, Subject: "state/view.db: drop table workspace_paths", Count: Unknown}},
					RewritesState: true,
				},
			},
			{
				Step: Step{Number: 3, Name: "chat-types", Phase: PostOpen, From: 1, To: 1},
				Plan: Plan{Actions: []Action{
					{Kind: Delete, Subject: "state/upgrades/chat-types", Count: 1},
					{Kind: Convert, Subject: "chats", Count: 12},
				}},
			},
		},
		Backup: BackupPlan{Required: true, Bytes: 2048, FreeBytes: 1 << 30},
	}
}

func TestRender_Format(t *testing.T) {
	lines := render(t, pendingReport())

	assert.Contains(t, lines, "home      /home/me/.crowbar")
	assert.Contains(t, lines, "layout    0 (legacy)   this binary knows up to 1")
	assert.Contains(t, lines, "lock      free")
	assert.Contains(t, lines, "last run  none")
	assert.Contains(t, lines, "2 pending steps, in execution order")
	assert.Contains(t, lines, "  #2 workspace-paths-table   pre-open   layout 0")
	assert.Contains(t, lines, "  #3 chat-types   post-open   layout 1")
	assert.Contains(t, lines, "    convert  state/view.db: drop table workspace_paths  ?")
	assert.Contains(t, lines, "    delete   state/upgrades/chat-types  1")
	assert.Contains(t, lines, "    convert  chats                      12")
	assert.Contains(t, lines, "state backup  required   2.0 KiB to copy, 1.0 GiB free")
	assert.NotContains(t, lines, "nothing to do")
}

func TestRender_Order(t *testing.T) {
	lines := render(t, pendingReport())

	at := func(line string) int {
		for i, l := range lines {
			if l == line {
				return i
			}
		}
		t.Fatalf("line %q is missing", line)
		return -1
	}
	assert.Less(t, at("lock      free"), at("last run  none"))
	assert.Less(t, at("last run  none"), at("2 pending steps, in execution order"))
	assert.Less(t, at("2 pending steps, in execution order"), at("  #2 workspace-paths-table   pre-open   layout 0"))
	assert.Less(t, at("  #2 workspace-paths-table   pre-open   layout 0"), at("  #3 chat-types   post-open   layout 1"))
	assert.Less(t, at("  #3 chat-types   post-open   layout 1"), at("state backup  required   2.0 KiB to copy, 1.0 GiB free"))
}

func TestRender_NothingToDoIsOneLine(t *testing.T) {
	lines := render(t, Report{Home: "/h", Layout: 1, Known: 1})

	assert.Contains(t, lines, "layout    1   this binary knows up to 1")
	assert.Contains(t, lines, "nothing to do")
	for _, line := range lines {
		assert.NotContains(t, line, "pending")
		assert.NotContains(t, line, "state backup")
	}
}

func TestRender_OnePendingStepIsSingular(t *testing.T) {
	r := pendingReport()
	r.Steps = r.Steps[:1]

	assert.Contains(t, render(t, r), "1 pending step, in execution order")
}

func TestRender_ABumpShowsBothLayouts(t *testing.T) {
	r := Report{Home: "/h", Known: 1, Steps: []StepPlan{
		{Step: Step{Number: 7, Name: "split-db", Phase: PreOpen, From: 0, To: 1}, Plan: Plan{}},
	}}

	lines := render(t, r)

	assert.Contains(t, lines, "  #7 split-db   pre-open   layout 0 -> 1")
	assert.Contains(t, lines, "    (nothing to change)")
}

func TestRender_StateBackup(t *testing.T) {
	r := pendingReport()

	t.Run("not needed", func(t *testing.T) {
		r.Backup = BackupPlan{}

		assert.Contains(t, render(t, r), "state backup  not needed")
	})

	t.Run("short of room", func(t *testing.T) {
		r.Backup = BackupPlan{Required: true, Bytes: 5 << 30, FreeBytes: 3 << 20}

		assert.Contains(t, render(t, r), "state backup  required   5.0 GiB to copy, 3.0 MiB free: not enough room, the boot will halt")
	})

	t.Run("room for the copy but not the headroom", func(t *testing.T) {
		r.Backup = BackupPlan{Required: true, Bytes: 10, FreeBytes: 10 + backupHeadroom - 1}

		assert.Contains(t, strings.Join(render(t, r), "\n"), "not enough room")
	})

	t.Run("room for the copy and its headroom", func(t *testing.T) {
		r.Backup = BackupPlan{Required: true, Bytes: 10, FreeBytes: 10 + backupHeadroom}

		assert.NotContains(t, strings.Join(render(t, r), "\n"), "not enough room")
	})
}

func TestRender_LockHeld(t *testing.T) {
	r := pendingReport()
	r.LockHeld = true

	assert.Contains(t, render(t, r), "lock      held by another crowbar process")
}

func TestRender_LastRun(t *testing.T) {
	r := pendingReport()

	t.Run("a finished run", func(t *testing.T) {
		r.Last = &RunSummary{
			ID: "r1", Version: "v1", Outcome: outcomeCompleted, LayoutFrom: 0, LayoutTo: 1,
			Start: minute(0), End: minute(1), Counts: Counts{"b": 2, "a": 1},
		}

		assert.Contains(t, render(t, r), "last run  r1   completed   layout 0 -> 1   version v1   counts a=1 b=2")
	})

	t.Run("a run that never ended", func(t *testing.T) {
		r.Last = &RunSummary{ID: "r2", LayoutFrom: 1, Start: minute(0), Counts: Counts{}}

		assert.Contains(t, render(t, r), "last run  r2   not finished   layout 1")
	})

	t.Run("a failed run says why", func(t *testing.T) {
		r.Last = &RunSummary{
			ID: "r3", Outcome: outcomeFailed, Error: "disk full", Start: minute(0), End: minute(1), Counts: Counts{},
		}

		assert.Contains(t, render(t, r), "last run  r3   failed   layout 0 -> 0   error: disk full")
	})
}

type failingWriter struct{}

func (failingWriter) Write(
	[]byte,
) (int, error) {
	return 0, errors.New("broken pipe")
}

func TestRender_ReturnsTheWriteError(t *testing.T) {
	err := pendingReport().Render(failingWriter{})

	require.ErrorContains(t, err, "broken pipe")
}
