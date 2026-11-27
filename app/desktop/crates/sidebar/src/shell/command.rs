mod clip_mode;
mod intent;
mod new_entry;
mod question;
mod row_actions;
mod sink;
mod stakes;

pub use clip_mode::ClipMode;
pub use intent::Intent;
pub use new_entry::NewEntry;
pub use question::Question;
pub use row_actions::RowActions;
pub use sink::Sink;
pub use stakes::Stakes;

use crate::shell::chats::DropMode;
use crate::shell::chats::RowId;
use crate::shell::daemons::HostId;
use crate::shell::git::AfterMerge;
use crate::shell::git::MergeStrategy;
use crate::shell::listing::FileTreeFlag;
use crate::shell::switcher::ProjectId;
use crate::shell::switcher::SpaceIcon;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Command {
    OpenSettings,
    OpenHostSettings(HostId),
    SelectHost(HostId),
    SelectProject(ProjectId),
    CreateSpace {
        name: String,
        folder: String,
        host: HostId,
    },
    ChooseSpaceFolder(HostId),
    DeleteProject(ProjectId),
    RenameProject {
        project: ProjectId,
        name: String,
    },
    RetryChats(ProjectId),
    OpenRow(RowId),
    Rename {
        row: RowId,
        name: String,
    },
    CommitRemoval(RowId),
    RemoveDiscardingWork(RowId),
    NewThread(ProjectId),
    NewChat(ProjectId),
    NewTerminalThread(ProjectId),
    Thread(RowId),
    Fork {
        parent: RowId,
        name: String,
    },
    MakeBranch {
        row: RowId,
        name: String,
    },
    CreateFolder(Option<RowId>),
    ImportRepo,
    ImportBranches(RowId),
    Lock(RowId),
    Unlock(RowId),
    Detach(RowId),
    Retry(RowId),
    Dismiss(RowId),
    MoveTo {
        row: RowId,
        target: RowId,
    },
    CancelMove(RowId),
    Drop {
        row: RowId,
        target: RowId,
        mode: DropMode,
    },
    OpenFile(String),
    PreviewFile(String),
    OpenReview {
        path: String,
    },
    SetFilesViewed {
        files: Vec<(String, Option<String>)>,
    },
    RevealFile {
        path: String,
    },
    ToggleDirectory(String),
    CreateFile {
        path: String,
    },
    CreateDirectory {
        path: String,
    },
    DuplicateFile {
        path: String,
    },
    RenameFile {
        path: String,
        new_path: String,
    },
    MoveFile {
        path: String,
        new_path: String,
    },
    DeleteFile {
        path: String,
    },
    CopyFile {
        path: String,
        destination: String,
    },
    RemoveFolderFromWorkspace {
        path: String,
    },
    CopyFileContent {
        path: String,
    },
    RefreshDirectory {
        path: String,
    },
    OpenAllFiles {
        path: String,
    },
    CollapseDirectory {
        path: String,
    },
    OpenTerminalAt {
        path: String,
    },
    UploadFile {
        path: String,
    },
    UploadPaths {
        path: String,
        files: Vec<std::path::PathBuf>,
    },
    GenerateImage {
        path: String,
    },
    CreateSpaceWithIcon {
        name: String,
        folder: String,
        host: HostId,
        icon: SpaceIcon,
    },
    SetSpaceIcon {
        project: ProjectId,
        icon: SpaceIcon,
    },
    SetRepoIcon {
        row: RowId,
        icon: SpaceIcon,
    },
    UseGithubAvatar(RowId),
    SetMergeStrategy(MergeStrategy),
    Merge {
        after: AfterMerge,
    },
    StopAndMerge,
    CommitChanges {
        message: String,
        paths: Vec<String>,
    },
    KillAgent(RowId),
    SetFileTreeFlag {
        flag: FileTreeFlag,
        on: bool,
    },
    CreateEnvTemplate {
        path: String,
        target: String,
    },
}
