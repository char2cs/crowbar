//! The sidebar of the Crowbar shell without a window: what the shell hands the sidebar and is
//! sent back ([`Source`], [`Sink`], [`Command`]) and every rule the views apply
//! (tree folds, inline edits, removal countdowns, drop policy, Files keys and clipboard, panel
//! geometry, peek zone, the preferences file). The `sidebarview` crate draws it with gpui.

mod error;
mod explorer;
mod fixtures;
mod frame;
mod shell;
mod tree;

pub use error::Error;
pub use explorer::{
    CopyNote, FileAction, FileClipboard, FileDialog, FileKey, FileProperties, MenuScope,
    ShellAnswers,
};
#[cfg(feature = "fixtures")]
pub use fixtures::DemoShell;
pub use frame::{
    DockSide, KeyStep, Mode, Panel, PeekZone, Preferences, Presentation, ResizeDrag, ResizeKey,
    Resizing, Visibility,
};
pub use shell::{
    ActiveSpace, Activity, AfterMerge, AvatarTint, ChangedFile, ChangedRow, Changes, ChangesSource,
    ClipMode, Command, Sink,
};
pub use shell::{
    DropMode, Expansion, FileAccess, FileDensity, FileKind, FileRow, FileTreeFlag, FileTreeView,
    FilesSource, Focus, FolderPhase, FolderSource, GitStatus, Host, HostId, HostKind, HostState,
    HostTone, IconRequests, IconTarget, IconTheme, IndentGuides, Intent, Lock, Mark, MergeBox,
    MergeFacts, MergeStrategy, MergeWork, MoveTarget, Nesting, NewEntry, Notice, Operation,
    OperationPhase, PaneFocus, PathText, Presence, Project, ProjectId, ProjectTree, Question,
    Reach, Row, RowActions, RowId, RowKind, RunningAgent, ScopeSync, Source, SpaceFacts, SpaceIcon,
    SpaceList, Titling, Workspace, WorkspaceStatus, Worktree,
};
pub use tree::{Confirmation, Deadline};
pub use tree::{
    DragArm, DropModes, EdgeScroll, Editing, FilesState, FolderTarget, Hold, MenuContext,
    RemovalKind, RemovalTarget, RemovalTray, Removals, TerminalThreads, Tree, VisibleRow,
    WorkAtRisk,
};
