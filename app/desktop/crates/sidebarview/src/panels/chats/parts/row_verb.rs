//! The hover buttons at the end of a tree row.

use ui::{IconName, IconWeight};

use sidebar::{Command, Intent, Lock, Question, Row, RowKind, WorkspaceStatus, Worktree};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowVerb {
    Detach,
    Retry,
    RepoMenu,
    Remove,
    Thread,
    Fork,
    MakeBranch,
}

impl RowVerb {
    pub(crate) fn for_row(row: &Row) -> Vec<Self> {
        let placeholder = is_placeholder(row);
        let held = row
            .workspace
            .as_ref()
            .is_some_and(|workspace| workspace.held_by_path.is_some());
        let is_branch = row.kind == RowKind::Branch;
        let leading = [
            (is_branch && held).then_some(Self::Detach),
            (is_branch && placeholder && !held).then_some(Self::Retry),
            (row.kind == RowKind::RepoHeader).then_some(Self::RepoMenu),
            is_removable(row).then_some(Self::Remove),
            (!placeholder).then_some(Self::Thread),
        ];
        let forks = (!placeholder).then(|| fork_and_branch(row));
        leading
            .into_iter()
            .flatten()
            .chain(forks.into_iter().flatten())
            .collect()
    }

    pub(crate) fn icon(self) -> (IconName, IconWeight) {
        match self {
            Self::Detach => (IconName::LinkBreak, IconWeight::Bold),
            Self::Retry => (IconName::ArrowClockwise, IconWeight::Bold),
            Self::RepoMenu => (IconName::DotsThree, IconWeight::Bold),
            Self::Remove => (IconName::X, IconWeight::Bold),
            Self::Thread => (IconName::ArrowElbowDownRight, IconWeight::Bold),
            Self::Fork | Self::MakeBranch => (IconName::GitBranch, IconWeight::Bold),
        }
    }

    pub(crate) fn intent(
        self,
        row: &Row,
    ) -> Option<Intent> {
        let id = row.id.clone();
        match self {
            Self::Detach => Some(Intent::Ask(Question::detach(
                id,
                &detached_branch(row),
                held_path(row),
            ))),
            Self::Retry => Some(Intent::Send(Command::Retry(id))),
            Self::RepoMenu => None,
            Self::Remove => Some(Intent::RequestRemoval(id)),
            Self::Thread => Some(Intent::Send(Command::Thread(id))),
            Self::Fork => Some(Intent::StartDraft(id)),
            Self::MakeBranch => Some(Intent::StartBranching(id)),
        }
    }
}

fn detached_branch(row: &Row) -> String {
    row.workspace
        .as_ref()
        .map_or_else(|| row.label.clone(), |workspace| workspace.branch.clone())
}

fn held_path(row: &Row) -> Option<&str> {
    row.workspace
        .as_ref()
        .and_then(|workspace| workspace.held_by_path.as_deref())
}

fn is_placeholder(row: &Row) -> bool {
    row.workspace
        .as_ref()
        .is_some_and(|workspace| workspace.status == WorkspaceStatus::Placeholder)
}

fn is_removable(row: &Row) -> bool {
    match row.kind {
        RowKind::Chat | RowKind::Folder => true,
        RowKind::Branch => row.lock == Lock::Unlocked,
        RowKind::RepoHeader => false,
    }
}

fn fork_and_branch(row: &Row) -> Vec<RowVerb> {
    let forks = match row.kind {
        RowKind::Folder => row.worktree == Worktree::Owned,
        RowKind::Branch => true,
        RowKind::Chat | RowKind::RepoHeader => false,
    };
    let can_branch =
        row.kind == RowKind::Chat && row.worktree == Worktree::Shared && row.pending_move.is_none();
    [
        forks.then_some(RowVerb::Fork),
        can_branch.then_some(RowVerb::MakeBranch),
    ]
    .into_iter()
    .flatten()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Workspace;

    fn branch(
        status: WorkspaceStatus,
        held: Option<&str>,
    ) -> Row {
        let mut row = Row::new("b", RowKind::Branch, 1, "feature");
        row.workspace = Some(Workspace {
            status,
            branch: "feature".into(),
            added: 0,
            deleted: 0,
            held_by_path: held.map(str::to_owned),
        });
        row
    }

    fn chat() -> Row {
        Row::new("c", RowKind::Chat, 2, "hi")
    }

    fn owned_chat() -> Row {
        let mut row = chat();
        row.worktree = Worktree::Owned;
        row
    }

    fn moving_chat() -> Row {
        let mut row = chat();
        row.pending_move = Some("src".into());
        row
    }

    fn repo_header() -> Row {
        Row::new("r", RowKind::RepoHeader, 0, "demo")
    }

    fn locked_branch() -> Row {
        let mut row = branch(WorkspaceStatus::New, None);
        row.lock = Lock::Locked;
        row
    }

    fn held_branch() -> Row {
        branch(WorkspaceStatus::New, Some("/tmp/x"))
    }

    fn placeholder_branch() -> Row {
        branch(WorkspaceStatus::Placeholder, None)
    }

    type Case = (&'static str, fn() -> Row, &'static [RowVerb]);

    const CASES: &[Case] = &[
        (
            "a repo thread can be removed, threaded and branched",
            chat,
            &[RowVerb::Remove, RowVerb::Thread, RowVerb::MakeBranch],
        ),
        (
            "a chat that owns a worktree cannot branch",
            owned_chat,
            &[RowVerb::Remove, RowVerb::Thread],
        ),
        (
            "a pending move blocks branching",
            moving_chat,
            &[RowVerb::Remove, RowVerb::Thread],
        ),
        (
            "the repo header has only the menu and thread",
            repo_header,
            &[RowVerb::RepoMenu, RowVerb::Thread],
        ),
        (
            "a locked branch cannot be removed",
            locked_branch,
            &[RowVerb::Thread, RowVerb::Fork],
        ),
        (
            "a holder leaves detach first",
            held_branch,
            &[
                RowVerb::Detach,
                RowVerb::Remove,
                RowVerb::Thread,
                RowVerb::Fork,
            ],
        ),
        (
            "a placeholder offers retry and nothing that needs a worktree",
            placeholder_branch,
            &[RowVerb::Retry, RowVerb::Remove],
        ),
    ];

    #[test]
    fn verbs_follow_the_row() {
        for (name, build, expected) in CASES {
            assert_eq!(RowVerb::for_row(&build()), *expected, "{name}");
        }
    }

    #[test]
    fn detaching_asks_with_the_branch_and_the_holder_path() {
        let row = branch(WorkspaceStatus::New, Some("/w/main"));
        let asked = RowVerb::Detach.intent(&row);
        let want = Question::detach(row.id.clone(), "feature", Some("/w/main"));
        assert_eq!(asked, Some(Intent::Ask(want)));
    }

    #[test]
    fn intents_act_on_the_row() {
        let row = branch(WorkspaceStatus::New, None);
        assert_eq!(RowVerb::RepoMenu.intent(&row), None, "repo menu is a popup");
        assert_eq!(
            RowVerb::Fork.intent(&row),
            Some(Intent::StartDraft(row.id.clone())),
            "fork starts a draft"
        );
    }
}
