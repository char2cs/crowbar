//! The sidebar of the Crowbar shell without a window: what the shell hands the sidebar and is
//! sent back ([`SidebarData`], [`CommandSink`], [`Command`]) and every rule the views apply
//! (tree folds, inline edits, removal countdowns, drop policy, Files keys and clipboard, panel
//! geometry, peek zone, the preferences file). The `sidebarview` crate draws it with gpui.

mod error;
mod explorer;
mod fixtures;
mod frame;
mod shell;
mod sidebar;
mod tree_state;

pub use error::Error;
pub use explorer::{
    FileAction, FileClipboard, FileDialog, FileKey, FileProperties, MenuScope, ShellAnswers,
};
#[cfg(feature = "fixtures")]
pub use fixtures::DemoData;
pub use frame::{
    DockSide, HEADER_HEIGHT, KeyStep, Mode, PEEK_INSET, Panel, PeekZone, Preferences, Presentation,
    ResizeDrag, ResizeKey, Resizing, Visibility,
};
pub use shell::{
    ActiveSpace, Activity, AfterMerge, AvatarTint, ChangedFile, Changes, ChangesSource, ClipMode,
    Command, CommandSink,
};
pub use shell::{
    DropMode, Expansion, FileAccess, FileDensity, FileKind, FileRow, FileTreeFlag, FileTreeView,
    FilesSource, Focus, GitStatus, Host, HostId, HostKind, HostState, HostTone, IconTheme, Intent,
    Lock, Mark, MergeBox, MergeFacts, MergeStrategy, MergeWork, MoveTarget, Nesting, NewEntry,
    Notice, Operation, OperationPhase, PaneFocus, Presence, Project, ProjectId, ProjectTree,
    Question, Reach, Row, RowActions, RowId, RowKind, RunningAgent, ScopeSync, SidebarData,
    SpaceFacts, SpaceIcon, SpaceList, Titling, Workspace, WorkspaceStatus, Worktree,
};
pub use sidebar::{absolute, base, join, parent};
pub use tree_state::{Confirmation, Deadline};
pub use tree_state::{
    DropModes, Editing, FilesState, FolderTarget, Hold, MenuContext, RemovalKind, RemovalTarget,
    RemovalTray, Removals, TerminalThreads, TreeState, VisibleRow, WorkAtRisk, drag_started, pull,
};
