# sidebar

Layer L1 (logic, gpui-free). See `docs/plans/2026-11-18-rust-tree.md` section 4.5.

## Owns

The sidebar's data contract with the shell and every rule its views apply, without a window: the
ports the shell implements (`Source` and the per-surface sources, `Sink`) and the data
in their signatures (rows, spaces, hosts, files, changes, merge box), plus the behaviour the views
drive: tree folds and visible rows, inline edits, removal countdowns and confirmations, drag
threshold, edge scrolling and drop policy, Files keys, clipboard, filter and folder chains, panel width, dock, peek zone and mode, and
the preferences file on disk. It has no single entry and no `src/sidebar.rs`; the root re-exports
types only, so the path arithmetic and the drag rules are methods of `PathText`, `DragArm` and
`EdgeScroll`.

## Public interface

- Contract: `Source` (read side, with `ActiveSpace`, `SpaceList`, `PaneFocus`, `FilesSource`,
  `ChangesSource`, `RowActions`), `Sink` and `Command` (write side), `Intent`, `Question`,
  `ClipMode`, `NewEntry`, `Error`.
- Chats tree rows: `Row`, `RowId`, `RowKind`, `MoveTarget`, `DropMode`, `ProjectTree`, `Notice`,
  `Reach`, `Mark`, `AvatarTint`, `Activity`, `Presence`, `Titling`, `Nesting`, `Expansion`, `Lock`,
  `Worktree`, `Workspace`, `WorkspaceStatus`, `Operation`, `OperationPhase`.
- Spaces and hosts: `Project`, `ProjectId`, `SpaceIcon`, `SpaceFacts`, `ScopeSync`, `Focus`,
  `RunningAgent`, `Host`, `HostId`, `HostKind`, `HostState`, `HostTone`.
- Files and git: `FileRow`, `FileKind`, `FileAccess`, `FileTreeView`, `FileTreeFlag`,
  `FileDensity`, `IconTheme`, `GitStatus`, `Changes`, `ChangedFile`, `MergeBox`, `MergeStrategy`,
  `MergeWork`, `AfterMerge`, `MergeFacts`, and `PathText` (a workspace-relative
  path: `join`, `parent`, `base`, `absolute`).
- Behaviour the views drive: `Tree`, `Editing`, `VisibleRow`, `FilesState`, `MenuContext`,
  `Removals`, `RemovalTray`, `Hold`, `Deadline`, `RemovalKind`,
  `RemovalTarget`, `Confirmation`, `WorkAtRisk`, `DropModes`, `FolderTarget`, `FileAction`,
  `FileClipboard`, `FileDialog`, `FileProperties`, `FileKey`, `ShellAnswers`, `MenuScope`,
  `Panel`, `DockSide`, `Visibility`, `Presentation`, `PeekZone`, `Mode`, `ResizeDrag`,
  `ResizeKey`, `Resizing`, `KeyStep`, `Preferences`, and the row drag rules `DragArm` (the drag
  threshold) and `EdgeScroll` (edge-scroll speed). `PeekZone` takes the header height and the card
  inset from the view, which owns those metrics.
- Feature `fixtures`: `DemoShell`, an in-memory workspace for tests.

## Depends on

- `atomicfile`: the preferences file is replaced all-or-nothing.
- External: `serde`/`serde_json` (preferences), `thiserror`. It never depends on gpui.
