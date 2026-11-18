//! The contract with the shell: the Source and Sink ports, commands and the data of every surface.

mod chats;
mod command;
mod daemons;
mod git;
mod listing;
mod sidebar_data;
mod switcher;

pub use chats::{
    Activity, AvatarTint, DropMode, Expansion, Lock, Mark, MoveTarget, Nesting, Notice, Operation,
    OperationPhase, Presence, ProjectTree, Reach, Row, RowId, RowKind, Titling, Workspace,
    WorkspaceStatus, Worktree,
};
pub use command::{ClipMode, Command, CommandSink, Intent, NewEntry, Question, RowActions};
pub use daemons::{Host, HostId, HostKind, HostState, HostTone};
pub use git::{
    AfterMerge, ChangedFile, Changes, ChangesSource, MergeBox, MergeFacts, MergeStrategy, MergeWork,
};
pub use listing::{
    FileAccess, FileDensity, FileKind, FileRow, FileTreeFlag, FileTreeView, FilesSource, GitStatus,
    IconTheme,
};
pub use sidebar_data::SidebarData;
pub use switcher::{
    ActiveSpace, Focus, PaneFocus, Project, ProjectId, RunningAgent, ScopeSync, SpaceFacts,
    SpaceIcon, SpaceList,
};
