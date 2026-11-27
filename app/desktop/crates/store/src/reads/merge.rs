use sidebar::{MergeBox, MergeStrategy, MergeWork};

use crate::live::State;
use crate::live::WorkspaceRecord;
use crate::reads::files::followed_workspace;
use crate::reads::merge_inputs::MergeInputs;

const MERGED: &str = "pr-merged";
const MERGE_KIND: &str = "workspace.merge_into_parent";
const IN_FLIGHT: [&str; 2] = ["accepted", "running"];
const FAILED: &str = "failed";
const CHAT_BUSY: &str = "chat_busy";

fn strategy_of(text: &str) -> MergeStrategy {
    match text {
        "squash" => MergeStrategy::Squash,
        "rebase" => MergeStrategy::Rebase,
        _ => MergeStrategy::Merge,
    }
}

pub(crate) fn strategy_wire(strategy: MergeStrategy) -> &'static str {
    match strategy {
        MergeStrategy::Merge => "merge",
        MergeStrategy::Squash => "squash",
        MergeStrategy::Rebase => "rebase",
    }
}

pub(crate) fn parent_branch(
    state: &State,
    workspace: &WorkspaceRecord,
) -> Option<String> {
    let id = Some(workspace.parent_id.as_str()).filter(|id| !id.is_empty())?;
    state
        .hosts
        .iter()
        .find_map(|slot| slot.store.workspaces.get(id))
        .map(|parent| parent.branch.clone())
}

fn operation_of<'a>(
    state: &'a State,
    workspace: &str,
) -> Option<&'a crate::live::OperationRecord> {
    state
        .hosts
        .iter()
        .flat_map(|slot| slot.store.operations.values())
        .filter(|op| op.kind == MERGE_KIND && op.subject.id == workspace && !op.dismissed)
        .max_by(|a, b| a.created_at.cmp(&b.created_at))
}

fn work_of(
    state: &State,
    operation: Option<&crate::live::OperationRecord>,
) -> MergeWork {
    if operation.is_some_and(|op| IN_FLIGHT.contains(&op.state.as_str())) {
        return MergeWork::Merging;
    }
    if state.panels.committing {
        return MergeWork::Committing;
    }
    MergeWork::Idle
}

pub(crate) fn merge_box(state: &State) -> Option<MergeBox> {
    let workspace = followed_workspace(state).filter(|workspace| !workspace.is_default)?;
    let parent = parent_branch(state, workspace);
    let uncommitted: Vec<String> = state.panels.statuses.keys().cloned().collect();
    let inputs = MergeInputs {
        parent: parent.as_deref(),
        uncommitted: uncommitted.len(),
        branch_files: state.panels.changes.files.len(),
        open_threads: state.panels.open_threads.total(),
        workspace,
    };
    let operation = operation_of(state, &workspace.id);
    let problem = operation.and_then(|op| op.error.as_ref());
    let busy = problem.filter(|problem| problem.code == CHAT_BUSY);
    let failed = operation.filter(|op| op.state == FAILED).and(problem);
    Some(MergeBox {
        merged: workspace.status == MERGED,
        blocked: inputs.blocked(),
        strategy: strategy_of(&workspace.merge_strategy),
        work: work_of(state, operation),
        working: busy.map(|problem| problem.message.clone()),
        error: failed
            .filter(|problem| problem.code != CHAT_BUSY)
            .map(|problem| problem.message.clone()),
        parent,
        uncommitted,
    })
}

#[cfg(test)]
mod tests {
    use sidebar::{ChangedFile, Changes};

    use super::*;
    use crate::test_support::state;

    #[test]
    fn the_followed_workspace_draws_a_merge_box_with_its_parent_and_reason() {
        let (mut state, _) = state();
        assert!(merge_box(&state).is_none());
        state.panels.follow(Some("hc".into()));
        let found = merge_box(&state).expect("a followed workspace has a merge box");
        assert!(found.blocked.is_some());
        assert_eq!(found.strategy, MergeStrategy::Merge);
        assert!(found.work == MergeWork::Idle && !found.merged);
        state.panels.committing = true;
        let committing = merge_box(&state).expect("still followed");
        assert_eq!(committing.work, MergeWork::Committing);
        let listed = ["README.md", "src/b.ts", "scratch.txt", "src/a/"];
        let files = listed.map(|path| ChangedFile {
            path: path.into(),
            ..ChangedFile::default()
        });
        let working = Changes {
            files: files.to_vec(),
            ..Changes::default()
        };
        state.panels.change("hc", (working, Vec::new()));
        let ordered = merge_box(&state).expect("still followed").uncommitted;
        assert_eq!(ordered, listed, "the commit form keeps git status order");
        state.panels.follow(Some("rdo".into()));
        assert!(
            merge_box(&state).is_none(),
            "a repo's default checkout has no merge card"
        );
    }
}
