# 14. Review and diff (REV)

### Open and shell

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-001 | Open Branch Review from tab-row "Review this branch" button | web/src/features/tabs/components/tab-bar.tsx:442 | workspace `feature/pricing-rounding`, click PR icon on tab row (button: see VT/IDE) | — | | | | | | | |
| REV-002 | Open from New Tab page "Review branch" (replaces new-tab page) | web/src/features/tabs/hooks/use-pane-new-tab-actions.ts:52 | ctrl+t > Review branch | — | | | | | | | |
| REV-003 | Open from sidebar Git panel / changed file: review opens and reveals that file | web/src/features/git/components/git-panel.tsx:42 | sidebar Git > src/pricing.ts (panel: see SB) | — | | | | | | | |
| REV-004 | Opening again focuses the existing "Branch Review" tab | web/src/features/panes/utils/pane-command-actions.ts:65 | REV-001 twice | — | | | | | | | |
| REV-005 | Header: title (owning chat title, else branch, else "Branch Review"), branch pill → base pill | web/src/features/git/components/branch-review-header.tsx:26 | after REV-001; shoot header | — | | | | | | | |
| REV-006 | Header stats "N files +A -D" once summary loaded (hidden before) | web/src/features/git/components/branch-review-header.tsx:45 | after REV-001 | — | | | | | | | |
| REV-007 | Segmented tabs "Conversation [count]" / "Files changed [count]"; badges hidden under 420px | web/src/features/git/components/branch-review-pane.tsx:127 | click each; rig size min + split to narrow (see SPL) | — | | | | | | | |
| REV-008 | Default view Conversation; chosen view persists on the tab | web/src/features/git/components/branch-review-pane.tsx:29 | pick Files changed, switch tab, back | — | | | | | | | |
| REV-009 | Arrow keys move between segmented tabs (activate on focus) | web/src/features/git/components/branch-review-pane.tsx:127 | Tab to segmented control, Right/Left | — | | | | | | | |
| REV-010 | Centred 960px column layout, responsive padding | web/src/features/git/components/branch-review-pane.tsx:119 | rig size standard / min | — | | | | | | | |

### Conversation tab

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-011 | Description card (markdown) shown only when non-empty | web/src/features/git/components/review-conversation-view.tsx:131 | Conversation tab. Seed: review description on feature workspace | — | | | | | | | |
| REV-012 | Thread card header: chevron, `path:line`, "Resolved" badge, "Jump to line" | web/src/features/git/components/review-conversation-view.tsx:36 | Conversation tab with 2 seeded threads | — | | | | | | | |
| REV-013 | Thread code snippet (selected lines highlighted, app light/dark) | web/src/features/git/components/review-thread-snippet.tsx:73 | Conversation tab; rig theme light / dark | — | | | | | | | |
| REV-014 | Snippet failure "Couldn’t load these lines." | web/src/features/git/components/review-thread-snippet.tsx:67 | Seed: thread anchored to lines no longer in patch / daemon read failure | — | | | | | | | |
| REV-015 | Collapse/expand thread via chevron ("Collapse thread"/"Expand thread") | web/src/features/git/components/review-conversation-view.tsx:37 | click chevron | — | | | | | | | |
| REV-016 | Resolved thread folds to its header, 60% opacity, until expanded | web/src/features/git/components/review-conversation-view.tsx:93 | resolve a thread (REV-028) | — | | | | | | | |
| REV-017 | Jump to line: switches to Files changed and scrolls to the thread's line | web/src/features/git/components/branch-review-pane.tsx:80 | click "Jump to line" | — | | | | | | | |
| REV-018 | General comment thread header "General comment" (no jump) | web/src/features/git/components/review-conversation-view.tsx:47 | post via REV-020, read card | — | | | | | | | |
| REV-019 | General composer: "Leave a comment", placeholder "Agents working on this branch pick this up on their own.", Comment disabled when empty | web/src/features/git/components/review-general-composer.tsx:38 | scroll to bottom of Conversation | — | | | | | | | |
| REV-020 | General composer post via button or mod+Enter; clears on success | web/src/features/git/components/review-general-composer.tsx:53 | type `looks good`, ctrl+Return | — | | | | | | | |
| REV-021 | General composer failure: inline red "Failed to post comment", text kept | web/src/features/git/hooks/use-review-thread-actions.ts:45 | Seed: daemon refusing comment POST | — | | | | | | | |
| REV-022 | No threads: only the composer (and description) shows | web/src/features/git/components/review-conversation-view.tsx:152 | workspace `main` review | — | | | | | | | |

### Thread item (both tabs)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-023 | Message row: avatar, name, @login, "agent" badge, "· relative time", markdown body | web/src/features/git/components/review-message-row.tsx:64 | any thread | — | | | | | | | |
| REV-024 | Agent message chat origin link "Open <chat>" / deleted-chat note | web/src/features/git/components/review-message-author.tsx:136 | Seed: agent-authored thread with chatId | — | | | | | | | |
| REV-025 | Message "Comment actions" ⋯ menu (Edit, Copy as markdown, Delete) on hover | web/src/features/git/components/review-message-row.tsx:87 | hover a message, click ⋯ (items: see MENU) | — | | | | | | | |
| REV-026 | Reply: read-only "Reply…" input opens composer (placeholder "Reply…", Cancel / Reply) | web/src/features/git/components/review-thread-item.tsx:215 | click Reply… | — | | | | | | | |
| REV-027 | Reply posted appends message; failure "Failed to post reply" | web/src/features/git/hooks/use-review-thread-actions.ts:59 | type reply, ctrl+Return | — | | | | | | | |
| REV-028 | Resolve button (outline in Conversation, filled in diff) → thread resolved, Reply input hidden | web/src/features/git/components/review-thread-item.tsx:144 | click Resolve | — | | | | | | | |
| REV-029 | Reopen button on resolved thread | web/src/features/git/components/review-thread-item.tsx:133 | expand resolved thread, click Reopen | — | | | | | | | |
| REV-030 | Resolve/Reopen disabled while submitting | web/src/features/git/components/review-thread-item.tsx:138 | click Resolve, shoot immediately | — | | | | | | | |
| REV-031 | Edit message: inline composer seeded with text; save / cancel; failure "Failed to edit comment" | web/src/features/git/components/review-message-row.tsx:120 | ⋯ > Edit on own message. Seed: thread authored by current identity | — | | | | | | | |
| REV-032 | Delete confirm dialog: "Delete this thread?" / "Delete this comment?" + description, Cancel / Delete | web/src/features/git/components/review-thread-item.tsx:240 | ⋯ > Delete (dialog: see DLG) | — | | | | | | | |
| REV-033 | Outdated thread: "Outdated" badge + "Show" collapsed row | web/src/features/git/components/review-thread-item.tsx:65 | Seed: thread on lines changed since posting | — | | | | | | | |

### Comment composer (Plate markdown)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-034 | Composer: optional title row, rich editor, Cancel / submit (disabled when empty), autofocus | web/src/features/panes/components/comment-composer.tsx:44 | open reply composer | — | | | | | | | |
| REV-035 | mod+Enter submits; bare Enter = new paragraph | web/src/features/editor/markdown/plate/comment/comment-editor-keys.ts:25 | type, Enter, type, ctrl+Return | — | | | | | | | |
| REV-036 | Escape cancels draft, but only closes link editor when it is open | web/src/features/editor/markdown/plate/comment/comment-editor-keys.ts:30 | select word, ctrl+k, Escape, Escape | — | | | | | | | |
| REV-037 | Selection floating toolbar (Bold, Italic, Underline, Strikethrough, Code, Link; no equation) | web/src/components/editor/plugins/mark-floating-toolbar-kit.tsx:21 | select text in composer | — | | | | | | | |
| REV-038 | Failed post keeps the draft text | web/src/features/git/components/diff/use-review-annotations.tsx:182 | Seed: daemon refusing comment POST | — | | | | | | | |

### Files changed (diff surface)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-039 | Loading: spinner "Loading branch diff" until files summary | web/src/features/git/components/review-diff-tab.tsx:112 | Files changed, shoot immediately | spinner | | | | | | | |
| REV-040 | Empty diff: dashed-file icon, "No changes between this branch and its parent.", Refresh link | web/src/features/git/components/review-diff-tab.tsx:120 | workspace `main` > review > Files changed | — | | | | | | | |
| REV-041 | Refresh re-loads the review | web/src/features/git/components/review-diff-tab.tsx:129 | click Refresh | — | | | | | | | |
| REV-042 | Toolbar in tab row: Side-by-side / Inline toggle group + "Find in diff" | web/src/features/git/components/review-diff-tab.tsx:138 | Files changed; shoot right of segmented tabs | — | | | | | | | |
| REV-043 | Split (side-by-side) default: old left, new right | web/src/features/git/components/diff/review-code-view.tsx:135 | Files changed | — | | | | | | | |
| REV-044 | Unified (inline) mode; choice persists as a setting across tabs | web/src/features/git/components/review-diff-tab.tsx:151 | click Inline; reopen review | — | | | | | | | |
| REV-045 | File cards with 12px gaps; sticky raised header while scrolling | web/src/features/git/components/diff/review-code-view.tsx:50 | scroll inside src/pricing.ts card | — | | | | | | | |
| REV-046 | File header: name, ± counts, "Viewed" checkbox, comment count chip | web/src/features/git/components/diff/review-code-view.tsx:285 | Files changed; shoot src/pricing.ts header | — | | | | | | | |
| REV-047 | Modified file diff with Shiki highlighting (2 highlighter workers) | web/src/features/git/components/diff/review-code-view.tsx:46 | src/pricing.ts card | — | | | | | | | |
| REV-048 | Added file (all lines added; split shows empty left) | web/src/features/git/components/diff/review-code-view.tsx:139 | src/pricing.test.ts card | — | | | | | | | |
| REV-049 | Removed file card | web/src/features/git/lib/review-placeholder.ts:41 | Seed: commit deleting a file | — | | | | | | | |
| REV-050 | Renamed file header old → new | web/src/features/git/components/diff/review-code-view.tsx:239 | Seed: commit renaming a file | — | | | | | | | |
| REV-051 | Added/removed line colours, intra-line (word) change highlights | web/src/features/git/components/diff/review-code-view.tsx:246 | src/pricing.ts changed line | — | | | | | | | |
| REV-052 | Hunk separator "N unchanged lines"; click expands 20 lines (file sides fetched on demand) | web/src/features/git/components/diff/review-code-view.tsx:59 | click a separator between hunks. Seed: change far enough from other lines to leave hidden context | — | | | | | | | |
| REV-053 | Expand context failure (a side missing) | web/src/features/git/components/diff/review-code-view.tsx:236 | expand in added file src/pricing.test.ts | — | | | | | | | |
| REV-054 | Viewed checkbox collapses file to header; unticking re-expands | web/src/features/git/components/diff/review-patch-header.tsx:34 | tick Viewed on src/pricing.ts | — | | | | | | | |
| REV-055 | Viewed state survives reload while file's kind/path/± unchanged | web/src/features/git/lib/review-viewed.ts:8 | tick Viewed, close and reopen review | — | | | | | | | |
| REV-056 | Header "Loading…" while a patch is fetched | web/src/features/git/components/diff/review-patch-header.tsx:65 | scroll fast to a not-yet-loaded file. Seed: many changed files | — | | | | | | | |
| REV-057 | Header "Could not load this diff" + Retry | web/src/features/git/components/diff/review-patch-header.tsx:68 | Seed: daemon patch failure | — | | | | | | | |
| REV-058 | Header "Diff truncated" + "Show all" | web/src/features/git/components/diff/review-patch-header.tsx:78 | Seed: single huge hunk over line cap | — | | | | | | | |
| REV-059 | Long-line / huge file rendered as plain text (no highlighting) | web/src/features/git/components/diff/review-code-view.tsx:38 | Seed: minified file / file over window budget | — | | | | | | | |
| REV-060 | Binary files block above diffs: rows "Binary file" (archive icon), not clickable | web/src/features/git/components/diff/review-binary-files.tsx:29 | Seed: binary file in branch diff | — | | | | | | | |
| REV-061 | Image row "Image" toggles image diff viewer (Removed / Added panes, zoom in/out, "No image") | web/src/features/git/components/diff/git-diff-image.tsx:17 | click Image row. Seed: changed image in branch diff | — | | | | | | | |
| REV-062 | Scroll position preserved across tab switch (per workspace/commit) | web/src/features/git/components/diff/review-code-view.tsx:228 | scroll, switch tab, back | — | | | | | | | |
| REV-063 | Fetching suspended while the pane is hidden | web/src/features/git/components/diff/review-code-view.tsx:81 | open review in hidden split pane (see SPL) | — | | | | | | | |

### Threads and comments on lines

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-064 | Thread annotation rendered under its line on the correct side (old=left, new=right) | web/src/features/git/components/diff/use-review-annotations.tsx:38 | Files changed, src/pricing.ts seeded threads | — | | | | | | | |
| REV-065 | Header comment chip "N comments" click reveals first thread | web/src/features/git/components/diff/review-patch-header.tsx:21 | click chat chip on src/pricing.ts header | — | | | | | | | |
| REV-066 | Hover a line: "+" gutter button "Comment on a line in <path>" | web/src/features/git/components/diff/use-review-annotations.tsx:252 | hover a line number | — | | | | | | | |
| REV-067 | Click "+": draft composer under line "Comment on line N of the new file" | web/src/features/git/components/diff/use-review-annotations.tsx:288 | click + on a new-side line | — | | | | | | | |
| REV-068 | Draft on old side: "... of the original file" | web/src/features/git/components/diff/use-review-annotations.tsx:287 | split view, + on a removed line | — | | | | | | | |
| REV-069 | Drag-select line range: draft "Comment on lines A–B of …", anchored to last line | web/src/features/git/components/diff/use-review-annotations.tsx:154 | drag over 3 line numbers | — | | | | | | | |
| REV-070 | Selection across both sides collapses to the start side/line | web/src/features/git/components/diff/use-review-annotations.tsx:151 | split view, drag from left to right column | — | | | | | | | |
| REV-071 | Clearing line selection drops the draft | web/src/features/git/components/diff/use-review-annotations.tsx:191 | start draft, click elsewhere in gutter | — | | | | | | | |
| REV-072 | Draft submit "Comment": becomes a thread in place; Conversation count increments | web/src/features/git/components/diff/use-review-annotations.tsx:171 | type, click Comment | — | | | | | | | |
| REV-073 | Draft Cancel removes composer | web/src/features/git/components/diff/use-review-annotations.tsx:169 | click Cancel | — | | | | | | | |
| REV-074 | Threads stream in live (posted by agent/MCP) | web/src/features/git/hooks/use-review-threads.ts:18 | Seed: external thread post while review open | — | | | | | | | |
| REV-075 | Resolved thread in diff at 60% opacity | web/src/features/git/components/review-thread-item.tsx:167 | resolve in diff | — | | | | | | | |

### Find in diff

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-076 | Open via toolbar magnifier (pressed state) or mod+f inside the diff | web/src/features/git/components/review-diff-tab.tsx:171 | click into diff, ctrl+f | — | | | | | | | |
| REV-077 | Search bar: "Find in diff…", prev/next (tooltips), Match case, Regex, Close (Esc) | web/src/features/git/components/diff/review-search-bar.tsx:137 | after REV-076 | — | | | | | | | |
| REV-078 | Debounced (200ms) daemon search: "Searching…" then "N matches" / "1 match" | web/src/features/git/components/diff/review-search-bar.tsx:281 | type `price` | — | | | | | | | |
| REV-079 | "No matches" | web/src/features/git/components/diff/review-search-bar.tsx:286 | type `zzz` | — | | | | | | | |
| REV-080 | "Showing the first N matches" when truncated | web/src/features/git/components/diff/review-search-bar.tsx:284 | Seed: diff with >200 hits | — | | | | | | | |
| REV-081 | Hit list rows (path, line, preview); click reveals line; active row highlighted; position "i/n" | web/src/features/git/components/diff/review-search-bar.tsx:229 | click 2nd hit | — | | | | | | | |
| REV-082 | Enter next / Shift+Enter previous wraps | web/src/features/git/components/diff/review-search-bar.tsx:143 | Enter ×3, shift+Return | — | | | | | | | |
| REV-083 | Search error inline alert ("Search failed" / invalid regex) | web/src/features/git/components/diff/review-search-bar.tsx:87 | toggle Regex, type `(` | — | | | | | | | |
| REV-084 | Escape / × closes search bar | web/src/features/git/components/diff/review-search-bar.tsx:146 | Escape | — | | | | | | | |

### Commit diff tab

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| REV-085 | Commit tab: own toolbar row "Commit <sha7>", "N changed file(s)", layout toggle + find | web/src/features/panes/components/commit-diff-pane.tsx:24 | sidebar Git > commit list > a commit (list: see SB) | — | | | | | | | |
| REV-086 | Commit tab loading "Loading commit diff" | web/src/features/git/components/review-diff-tab.tsx:115 | open commit, shoot immediately | spinner | | | | | | | |
| REV-087 | Commit with no file changes: "This commit changed no files." + Refresh (remounts) | web/src/features/panes/components/commit-diff-pane.tsx:30 | Seed: empty commit | — | | | | | | | |
| REV-088 | Reopening same commit focuses its tab (one per commit per workspace) | web/src/features/panes/stores/slices/new-buffer.ts:59 | open same commit twice | — | | | | | | | |

Skipped:
- `web/src/features/git/components/git-panel.tsx`, `changed-files-tree.tsx`, `commit-changes-form.tsx`, `review-merge-box.tsx`: sidebar Git panel incl. stage/commit/merge (see SB); the review surface has no stage/unstage/discard.
- Branch/commit compare selectors: none in the Tauri review UI (base is the workspace's parent branch).
- `branchReview.diffStatus` 'error' (`branch-review-pane.tsx:103`): stored, never rendered.
- Keyboard navigation between files/hunks/threads: none beyond find-in-diff Enter/Shift+Enter.
- `get_review_scope`: MCP tool only, no UI.
- `web/src/features/git/api/*`, `stores/git-refresh.ts`, `utils/*`: data plumbing, no own UI.
- Diff worker-pool fallback (`review-code-view.tsx:94`): only when Worker construction throws; not reachable in the desktop app.
