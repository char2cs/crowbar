//! A small in-memory shell for the gallery and tests: it serves a fixed workspace and applies the
//! commands the sidebar sends.

use std::cell::RefCell;

use crate::activity::Activity;
use crate::avatar_tint::AvatarTint;
use crate::changed_file::ChangedFile;
use crate::changes::Changes;
use crate::command::Command;
use crate::command_sink::CommandSink;
use crate::error::Error;
use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_row::FileRow;
use crate::focus::Focus;
use crate::git_status::GitStatus;
use crate::host::Host;
use crate::host_id::HostId;
use crate::host_kind::HostKind;
use crate::host_state::HostState;
use crate::lock::Lock;
use crate::mark::Mark;
use crate::move_target::MoveTarget;
use crate::nesting::Nesting;
use crate::presence::Presence;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::scope_sync::ScopeSync;
use crate::sidebar_data::SidebarData;
use crate::titling::Titling;
use crate::workspace::Workspace;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

/// The fixed workspace of the gallery; rows can be renamed and removed through the sink.
#[derive(Debug)]
pub struct DemoData {
    rows: RefCell<Vec<Row>>,
    active: RefCell<ProjectId>,
    spaces: usize,
    hosts: usize,
    sync: ScopeSync,
    focus: Focus,
}

impl DemoData {
    /// Two spaces and one host with a locked `main` and one provisional chat that owns a worktree,
    /// as in the reference capture.
    pub fn reference() -> Self {
        Self::with_rows(reference_rows(), (2, 1))
    }

    /// Several spaces and a tree that uses every row kind, state and badge.
    pub fn rich() -> Self {
        Self::with_rows(rich_rows(), (3, 2))
    }

    fn with_rows(rows: Vec<Row>, (spaces, hosts): (usize, usize)) -> Self {
        Self {
            rows: RefCell::new(rows),
            active: RefCell::new(ProjectId::from("inv")),
            spaces,
            hosts,
            sync: ScopeSync::Ready,
            focus: Focus::Repository,
        }
    }

    /// What the focused pane shows; only a repository enables every mode.
    #[must_use]
    pub fn with_focus(mut self, focus: Focus) -> Self {
        self.focus = focus;
        self
    }

    /// The reference workspace before the projects scope has synced, which shows the skeleton.
    pub fn loading() -> Self {
        Self {
            sync: ScopeSync::Pending,
            ..Self::reference()
        }
    }
}

fn branch(id: &str, depth: u8, label: &str, workspace: Workspace) -> Row {
    let mut row = Row::new(id, RowKind::Branch, depth, label);
    row.workspace = Some(workspace);
    row
}

fn landed(status: WorkspaceStatus, branch: &str, added: u32, deleted: u32) -> Workspace {
    Workspace {
        status,
        branch: branch.to_owned(),
        added,
        deleted,
        held_by_path: None,
    }
}

fn repo_header(id: &str, label: &str, letter: char, tint: AvatarTint) -> Row {
    let mut row = Row::new(id, RowKind::RepoHeader, 0, label);
    row.mark = Some(Mark::Letter { letter, tint });
    row.nesting = Nesting::Parent;
    row
}

fn reference_rows() -> Vec<Row> {
    let mut main = branch(
        "main",
        1,
        "main",
        landed(WorkspaceStatus::Locked, "main", 0, 0),
    );
    main.lock = Lock::Locked;
    let mut chat = Row::new("chat", RowKind::Branch, 1, "Untitled chat");
    chat.titling = Titling::Provisional;
    chat.presence = Presence::Open;
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
    working.workspace = Some(landed(WorkspaceStatus::PrOpen, "sync-loop", 42, 7));
    let mut folder = Row::new("folder", RowKind::Folder, 1, "Experiments");
    folder.nesting = Nesting::Parent;
    let child = branch(
        "child",
        2,
        "spike",
        landed(WorkspaceStatus::PrMerged, "spike", 3, 1),
    );
    rows.extend([working, folder, child]);
    rows
}

impl SidebarData for DemoData {
    fn sync(&self) -> ScopeSync {
        self.sync
    }

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
                host: HostId::from("local"),
            })
            .collect()
    }

    fn active_project(&self) -> Option<ProjectId> {
        Some(self.active.borrow().clone())
    }

    fn tree(&self, project: &ProjectId) -> ProjectTree {
        let rows = if project == &*self.active.borrow() {
            self.rows.borrow().clone()
        } else {
            Vec::new()
        };
        ProjectTree {
            rows,
            ..ProjectTree::default()
        }
    }

    fn focus(&self) -> Focus {
        self.focus
    }

    fn move_targets(&self, _row: &RowId) -> Vec<MoveTarget> {
        Vec::new()
    }

    fn can_start_terminal_thread(&self) -> bool {
        true
    }

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

    fn changes(&self) -> Changes {
        Changes {
            branch: "inv-rb".into(),
            parent: Some("main".into()),
            files: vec![ChangedFile {
                path: "src/lib.rs".into(),
                status: GitStatus::Added,
                added: 1,
                deleted: 0,
            }],
        }
    }
}

impl DemoData {
    fn rename(&self, row: &RowId, name: &str) -> Result<(), Error> {
        let mut rows = self.rows.borrow_mut();
        if rows
            .iter()
            .any(|other| &other.id != row && other.label == name)
        {
            return Err(Error::Refused {
                reason: format!("\"{name}\" already exists"),
            });
        }
        for found in rows.iter_mut().filter(|other| &other.id == row) {
            name.clone_into(&mut found.label);
            found.titling = Titling::Final;
        }
        Ok(())
    }
}

impl CommandSink for DemoData {
    fn dispatch(&self, command: Command) -> Result<(), Error> {
        match command {
            Command::SelectProject(project) => *self.active.borrow_mut() = project,
            Command::Rename { row, name } => return self.rename(&row, &name),
            Command::CommitRemoval(row) => self.rows.borrow_mut().retain(|other| other.id != row),
            _ => {},
        }
        Ok(())
    }
}

#[cfg(test)]
#[path = "demo_data_tests.rs"]
mod tests;
