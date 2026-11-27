use std::cell::RefCell;

use crate::error::Error;
use crate::shell::{
    ActiveSpace, Activity, AvatarTint, ChangedFile, Changes, ChangesSource, Command, Expansion,
    FileAccess, FileKind, FileRow, FilesSource, Focus, FolderPhase, FolderSource, GitStatus, Host,
    HostChoice, HostId, HostKind, HostState, IconTarget, Lock, Mark, MergeBox, MoveTarget, Nesting,
    PaneFocus, Project, ProjectId, ProjectTree, Requests, Row, RowActions, RowId, RowKind,
    ScopeSync, Sink, SpaceList, Titling, Workspace, WorkspaceStatus, Worktree,
};

const RICH_COUNTS: (usize, usize) = (3, 2);
const MAIN: &str = "main";
const SYNC_LOOP_ADDED: u32 = 42;
const SYNC_LOOP_DELETED: u32 = 7;
const SPIKE_ADDED: u32 = 3;
const SPIKE_DELETED: u32 = 1;

#[derive(Debug)]
pub struct DemoShell {
    rows: RefCell<Vec<Row>>,
    active: RefCell<ProjectId>,
    spaces: usize,
    hosts: usize,
    shown: HostId,
    sync: ScopeSync,
    focus: Focus,
    refused: Vec<Command>,
    removable: Vec<String>,
    merge: Option<MergeBox>,
    git_error: Option<String>,
    busy_icons: Vec<IconTarget>,
    adding: Vec<String>,
}

impl DemoShell {
    pub fn reference() -> Self {
        Self::with_rows(reference_rows(), (2, 1))
    }

    pub fn rich() -> Self {
        Self::with_rows(rich_rows(), RICH_COUNTS)
    }

    pub fn empty() -> Self {
        Self::with_rows(Vec::new(), (0, 1))
    }

    pub fn loading() -> Self {
        Self {
            sync: ScopeSync::Pending,
            ..Self::reference()
        }
    }

    pub fn showing_down_host() -> Self {
        Self {
            spaces: 0,
            sync: ScopeSync::Pending,
            ..Self::on_down_host()
        }
    }

    pub fn on_down_host() -> Self {
        Self {
            shown: HostId::from("devbox"),
            ..Self::rich()
        }
    }

    fn with_rows(
        rows: Vec<Row>,
        (spaces, hosts): (usize, usize),
    ) -> Self {
        Self {
            rows: RefCell::new(rows),
            active: RefCell::new(ProjectId::from("inv")),
            spaces,
            hosts,
            shown: HostId::from("local"),
            sync: ScopeSync::Ready,
            focus: Focus::Repository,
            refused: Vec::new(),
            removable: Vec::new(),
            merge: None,
            git_error: None,
            busy_icons: Vec::new(),
            adding: Vec::new(),
        }
    }

    #[must_use]
    pub fn with_focus(
        mut self,
        focus: Focus,
    ) -> Self {
        self.focus = focus;
        self
    }

    #[must_use]
    pub fn refusing(
        mut self,
        command: Command,
    ) -> Self {
        self.refused.push(command);
        self
    }

    #[must_use]
    pub fn with_removable_folder(
        mut self,
        path: &str,
    ) -> Self {
        self.removable.push(path.to_owned());
        self
    }

    #[must_use]
    pub fn with_merge_box(
        mut self,
        merge: MergeBox,
    ) -> Self {
        self.merge = Some(merge);
        self
    }

    #[must_use]
    pub fn with_git_error(
        mut self,
        reason: &str,
    ) -> Self {
        self.git_error = Some(reason.to_owned());
        self
    }

    #[must_use]
    pub fn with_busy_icon(
        mut self,
        target: IconTarget,
    ) -> Self {
        self.busy_icons.push(target);
        self
    }

    #[must_use]
    pub fn with_adding(
        mut self,
        folder: &str,
    ) -> Self {
        self.adding.push(folder.to_owned());
        self
    }

    fn rename(
        &self,
        row: &RowId,
        name: &str,
    ) -> Result<(), Error> {
        let mut rows = self.rows.borrow_mut();
        let taken = rows
            .iter()
            .any(|other| &other.id != row && other.label == name);
        if taken {
            return Err(already_exists(name));
        }
        for found in rows.iter_mut().filter(|other| &other.id == row) {
            name.clone_into(&mut found.label);
            found.titling = Titling::Final;
        }
        Ok(())
    }
}

fn already_exists(name: &str) -> Error {
    Error::Refused {
        reason: format!("\"{name}\" already exists"),
    }
}

impl Sink for DemoShell {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
        match command {
            Command::SelectProject(project) => *self.active.borrow_mut() = project,
            Command::Rename { row, name } => return self.rename(&row, &name),
            Command::CommitRemoval(row) | Command::RemoveDiscardingWork(row) => {
                self.rows.borrow_mut().retain(|other| other.id != row);
            },
            _ => {},
        }
        Ok(())
    }
}
fn branch(
    id: &str,
    depth: u8,
    label: &str,
    workspace: Workspace,
) -> Row {
    let mut row = Row::new(id, RowKind::Branch, depth, label);
    row.workspace = Some(workspace);
    row
}

fn landed(
    status: WorkspaceStatus,
    branch: &str,
    added: u32,
    deleted: u32,
) -> Workspace {
    Workspace {
        status,
        branch: branch.to_owned(),
        added,
        deleted,
        held_by_path: None,
    }
}

fn repo_header(
    id: &str,
    label: &str,
    letter: char,
    tint: AvatarTint,
) -> Row {
    let mut row = Row::new(id, RowKind::RepoHeader, 0, label);
    row.mark = Some(Mark::Letter { letter, tint });
    row.nesting = Nesting::Parent;
    row
}

fn reference_rows() -> Vec<Row> {
    let mut main = branch(MAIN, 1, MAIN, landed(WorkspaceStatus::Locked, MAIN, 0, 0));
    main.lock = Lock::Locked;
    let mut chat = Row::new("chat", RowKind::Branch, 1, "Untitled chat");
    chat.titling = Titling::Provisional;
    chat.worktree = Worktree::Owned;
    chat.workspace = Some(landed(WorkspaceStatus::New, "inv-rb", 1, 0));
    vec![
        repo_header("demo", "demo", 'D', AvatarTint::Slate),
        main,
        chat,
    ]
}

fn rich_rows() -> Vec<Row> {
    let mut rows = reference_rows();
    let mut working = Row::new("working", RowKind::Chat, 1, "Refactor the sync loop");
    working.activity = Activity::Working;
    working.workspace = Some(landed(
        WorkspaceStatus::PrOpen,
        "sync-loop",
        SYNC_LOOP_ADDED,
        SYNC_LOOP_DELETED,
    ));
    let mut folder = Row::new("folder", RowKind::Folder, 1, "Experiments");
    folder.nesting = Nesting::Parent;
    let child = branch(
        "child",
        2,
        "spike",
        landed(
            WorkspaceStatus::PrMerged,
            "spike",
            SPIKE_ADDED,
            SPIKE_DELETED,
        ),
    );
    rows.extend([working, folder, child]);
    rows
}

impl SpaceList for DemoShell {
    fn sync(&self) -> ScopeSync {
        self.sync
    }

    fn projects(&self) -> Vec<Project> {
        let names = [
            ("inv", Mark::Fallback),
            ("website", Mark::Fallback),
            (
                "api",
                Mark::Letter {
                    letter: 'A',
                    tint: AvatarTint::Emerald,
                },
            ),
        ];
        names
            .into_iter()
            .take(self.spaces)
            .map(|(name, mark)| Project {
                id: ProjectId::from(name),
                name: name.to_owned(),
                mark,
                host: self.shown.clone(),
            })
            .collect()
    }
}

impl HostChoice for DemoShell {
    fn hosts(&self) -> Vec<Host> {
        let local = Host {
            id: HostId::from("local"),
            name: "This computer".into(),
            kind: HostKind::Local,
            state: HostState::Up,
            endpoint: None,
        };
        if self.hosts < 2 {
            return vec![local];
        }
        let remote = Host {
            id: HostId::from("devbox"),
            name: "devbox".into(),
            kind: HostKind::Remote,
            state: HostState::Down,
            endpoint: Some("devbox.internal:7421".into()),
        };
        vec![local, remote]
    }

    fn shown_host(&self) -> Option<HostId> {
        Some(self.shown.clone())
    }
}

impl ActiveSpace for DemoShell {
    fn active_project(&self) -> Option<ProjectId> {
        Some(self.active.borrow().clone())
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        let mut tree = ProjectTree::default();
        if project == &*self.active.borrow() {
            tree.rows.clone_from(&self.rows.borrow());
        }
        tree
    }
}

impl PaneFocus for DemoShell {
    fn focus(&self) -> Focus {
        self.focus
    }

    fn can_start_terminal_thread(&self) -> bool {
        true
    }
}

impl RowActions for DemoShell {
    fn move_targets(
        &self,
        _row: &RowId,
    ) -> Vec<MoveTarget> {
        Vec::new()
    }

    fn serves(
        &self,
        command: &Command,
    ) -> bool {
        !self.refused.contains(command)
    }

    fn can_remove_folder(
        &self,
        path: &str,
    ) -> bool {
        self.removable.iter().any(|folder| folder == path)
    }
}

impl FilesSource for DemoShell {
    fn files(&self) -> Vec<FileRow> {
        let file = |path: &str, name: &str, depth, kind, status| FileRow {
            path: path.to_owned(),
            name: name.to_owned(),
            depth,
            kind,
            status,
        };
        vec![
            file(
                "src",
                "src",
                0,
                FileKind::Directory(Expansion::Expanded),
                GitStatus::Clean,
            ),
            file(
                "src/main.rs",
                "main.rs",
                1,
                FileKind::File,
                GitStatus::Modified,
            ),
            file("src/lib.rs", "lib.rs", 1, FileKind::File, GitStatus::Added),
            file(
                "README.md",
                "README.md",
                0,
                FileKind::File,
                GitStatus::Clean,
            ),
        ]
    }

    fn workspace_root(&self) -> Option<String> {
        Some("/work/demo".to_owned())
    }
}

impl ChangesSource for DemoShell {
    fn merge_box(&self) -> Option<MergeBox> {
        self.merge.clone()
    }

    fn changes(&self) -> Changes {
        let file = ChangedFile {
            path: "src/lib.rs".into(),
            status: GitStatus::Added,
            added: 1,
            deleted: 0,
            ..ChangedFile::default()
        };
        Changes {
            branch: "inv-rb".into(),
            parent: Some(MAIN.into()),
            files: vec![file],
            error: self.git_error.clone(),
            ..Changes::default()
        }
    }
}

impl FileAccess for DemoShell {}

impl FolderSource for DemoShell {
    fn folder_phase(&self) -> FolderPhase {
        FolderPhase::Listed
    }
}

impl Requests for DemoShell {
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool {
        self.busy_icons.contains(target)
    }

    fn adding(
        &self,
        folder: &str,
    ) -> bool {
        self.adding.iter().any(|adding| adding == folder)
    }
}
