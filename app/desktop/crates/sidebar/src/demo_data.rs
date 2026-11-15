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
use crate::merge_box::MergeBox;
use crate::move_target::MoveTarget;
use crate::nesting::Nesting;
use crate::presence::Presence;
use crate::project::Project;
use crate::project_id::ProjectId;
use crate::project_tree::ProjectTree;
use crate::row::Row;
use crate::row_id::RowId;
use crate::row_kind::RowKind;
use crate::running_agent::RunningAgent;
use crate::scope_sync::ScopeSync;
use crate::sidebar_data::SidebarData;
use crate::titling::Titling;
use crate::workspace::Workspace;
use crate::workspace_status::WorkspaceStatus;
use crate::worktree::Worktree;

const RICH_COUNTS: (usize, usize) = (3, 2);
const SYNC_LOOP_ADDED: u32 = 42;
const SYNC_LOOP_DELETED: u32 = 7;
const SPIKE_ADDED: u32 = 3;
const SPIKE_DELETED: u32 = 1;

/// The fixed workspace of the gallery; rows can be renamed and removed through the sink.
#[derive(Debug)]
pub struct DemoData {
    rows: RefCell<Vec<Row>>,
    active: RefCell<ProjectId>,
    spaces: usize,
    hosts: usize,
    sync: ScopeSync,
    focus: Focus,
    refused: Vec<Command>,
    removable: Vec<String>,
    merge: Option<MergeBox>,
    running: Vec<RunningAgent>,
}

impl DemoData {
    /// Two spaces and one host with a locked `main` and one provisional chat that owns a worktree,
    /// as in the reference capture.
    pub fn reference() -> Self {
        Self::with_rows(reference_rows(), (2, 1))
    }

    /// Several spaces and a tree that uses every row kind, state and badge.
    pub fn rich() -> Self {
        Self::with_rows(rich_rows(), RICH_COUNTS)
    }

    pub(crate) fn with_rows(
        rows: Vec<Row>,
        (spaces, hosts): (usize, usize),
    ) -> Self {
        Self {
            rows: RefCell::new(rows),
            active: RefCell::new(ProjectId::from("inv")),
            spaces,
            hosts,
            sync: ScopeSync::Ready,
            focus: Focus::Repository,
            refused: Vec::new(),
            removable: Vec::new(),
            merge: None,
            running: Vec::new(),
        }
    }

    /// What the focused pane shows; only a repository enables every mode.
    #[must_use]
    pub fn with_focus(
        mut self,
        focus: Focus,
    ) -> Self {
        self.focus = focus;
        self
    }

    /// The same data with a shell that declines `command`, which hides its menu entries.
    #[must_use]
    pub fn refusing(
        mut self,
        command: Command,
    ) -> Self {
        self.refused.push(command);
        self
    }

    /// The same data with `path` as one of several folders of the workspace.
    #[must_use]
    pub fn with_removable_folder(
        mut self,
        path: &str,
    ) -> Self {
        self.removable.push(path.to_owned());
        self
    }

    /// The same data with `merge` as the merge box of the focused workspace.
    #[must_use]
    pub fn with_merge_box(
        mut self,
        merge: MergeBox,
    ) -> Self {
        self.merge = Some(merge);
        self
    }

    /// The same data with a running agent on `row` titled `title`.
    #[must_use]
    pub fn with_running_agent(
        mut self,
        row: &str,
        title: &str,
    ) -> Self {
        self.running.push(RunningAgent {
            row: RowId::from(row),
            title: title.to_owned(),
            viewers: 0,
        });
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

    fn focus(&self) -> Focus {
        self.focus
    }

    fn move_targets(
        &self,
        _row: &RowId,
    ) -> Vec<MoveTarget> {
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

    fn workspace_root(&self) -> Option<String> {
        Some("/work/demo".to_owned())
    }

    fn merge_box(&self) -> Option<MergeBox> {
        self.merge.clone()
    }

    fn running_agents(&self) -> Vec<RunningAgent> {
        self.running.clone()
    }

    fn changes(&self) -> Changes {
        let file = ChangedFile {
            path: "src/lib.rs".into(),
            status: GitStatus::Added,
            added: 1,
            deleted: 0,
        };
        Changes {
            branch: "inv-rb".into(),
            parent: Some("main".into()),
            files: vec![file],
        }
    }
}

fn already_exists(name: &str) -> Error {
    Error::Refused {
        reason: format!("\"{name}\" already exists"),
    }
}

impl DemoData {
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

impl CommandSink for DemoData {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
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
mod tests {
    use super::*;

    #[test]
    fn renaming_to_a_taken_name_is_refused_and_leaves_the_tree_alone() {
        let data = DemoData::reference();
        let refused = data.dispatch(Command::Rename {
            row: RowId::from("chat"),
            name: "main".into(),
        });
        assert!(
            matches!(refused, Err(Error::Refused { .. })),
            "got {refused:?}"
        );
        let tree = data.tree(&ProjectId::from("inv"));
        assert!(
            tree.rows.iter().any(|row| row.label == "Untitled chat"),
            "label kept"
        );
    }

    #[test]
    fn committing_a_removal_drops_the_row() {
        let data = DemoData::reference();
        data.dispatch(Command::CommitRemoval(RowId::from("chat")))
            .expect("accepted");
        assert!(
            data.tree(&ProjectId::from("inv"))
                .rows
                .iter()
                .all(|row| row.id != RowId::from("chat"))
        );
    }

    type CountCase = (&'static str, fn() -> DemoData, usize, usize);

    const COUNT_CASES: &[CountCase] = &[
        ("the reference data", DemoData::reference, 2, 1),
        ("the rich data", DemoData::rich, 3, 2),
    ];

    #[test]
    fn the_data_lists_its_spaces_and_hosts() {
        for (name, build, spaces, hosts) in COUNT_CASES {
            let data = build();
            assert_eq!(data.projects().len(), *spaces, "{name}: spaces");
            assert_eq!(data.hosts().len(), *hosts, "{name}: hosts");
        }
    }
}
