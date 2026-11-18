//! Plans that file a row somewhere else.

mod placement;
mod slot;

use daemonapi::Call;
use serde_json::json;
use sidebar::DropMode;

use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::relocate::placement::slot_for;
use crate::plan::relocate::slot::Slot;
use crate::reads::{active_project, host_of, tree};
use crate::target::Kind;
use crate::target::resolve;

pub(crate) fn drop_row(
    state: &State,
    (row, target): (&str, &str),
    mode: DropMode,
) -> Result<Plan, PlanError> {
    let project = active_project(state).ok_or(PlanError::NoSpaceOpen)?;
    let rows = tree(state, &project).rows;
    let slot = slot_for(&rows, (row, target), mode).ok_or(PlanError::CannotGoThere)?;
    place(state, row, &slot)
}

fn place(
    state: &State,
    row: &str,
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let moved = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    let host = host_of(state, &moved.project)
        .map(|held| held.spec.id.clone())
        .ok_or(PlanError::SpaceNotLoaded)?;
    let (project, repo) = (&moved.project, &moved.repo);
    let body = json!({"parentId": slot.parent, "order": slot.index});
    let path = match &moved.kind {
        Kind::Folder => format!("{}/{row}", daemonapi::Route::folders(project, repo)),
        Kind::Branch {
            workspace,
            locked: true,
            ..
        } => {
            format!(
                "{}/workspaces/{workspace}/placement",
                daemonapi::Route::scope(project, repo)
            )
        },
        Kind::Branch { .. } | Kind::Chat { .. } => {
            format!(
                "{}/{}/placement",
                daemonapi::Route::chats(project, repo),
                moved.chat()?
            )
        },
        Kind::RepoHeader { .. } => {
            return Err(PlanError::RepositoryNotDraggable);
        },
    };
    Ok(Plan::of(&host, vec![Call::patch(path).json(body)]))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn dropping_a_thread_into_a_folder_patches_its_placement() {
        let (state, _) = state();
        let plan = drop_row(&state, ("t", "f"), DropMode::Into).unwrap();
        assert_eq!(
            plan.calls[0].path,
            "/v0/projects/p/repos/r/chats/t/placement"
        );
        assert_eq!(plan.calls[0].body.clone().unwrap()["parentId"], "f");
    }

    #[test]
    fn a_repository_header_cannot_be_dragged() {
        let (state, _) = state();
        assert!(drop_row(&state, ("rdo", "f"), DropMode::Into).is_err());
    }
}
