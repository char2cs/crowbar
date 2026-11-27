//! Plans that file a row somewhere else.
//!
//! Spec: develop `web/src/components/sidebar/lib/sidebar-drop-dispatch.ts`.

mod branch_drop;
mod chat_drop;
mod container;
mod folder_drop;
mod placement;
mod slot;

use daemonapi::{Call, Route};
use serde_json::json;
use sidebar::{DropMode, Row};

use crate::live::HostSlot;
use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::relocate::branch_drop::BranchDrop;
use crate::plan::relocate::placement::slot_for;
use crate::plan::relocate::slot::Slot;
use crate::reads::{active_project, host_of, tree};
use crate::rows::target::{Kind, Target, resolve};

pub(crate) fn drop_row(
    state: &State,
    (row, target): (&str, &str),
    mode: DropMode,
) -> Result<Plan, PlanError> {
    let project = active_project(state).ok_or(PlanError::NoSpaceOpen)?;
    let mut rows = tree(state, &project).rows;
    rows.retain(|row| row.operation.is_none());
    let slot = slot_for(&rows, (row, target), mode).ok_or(PlanError::CannotGoThere)?;
    place(state, (&rows, row), &slot)
}

fn place(
    state: &State,
    (rows, row): (&[Row], &str),
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let moved = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    match &moved.kind {
        Kind::Folder => folder_drop::plan(state, (rows, &moved), slot),
        Kind::Branch { workspace, .. } => branch(state, (&moved, workspace), slot),
        Kind::Chat { .. } => chat_drop::plan(state, (rows, &moved), slot),
        Kind::RepoHeader { .. } => repository(state, &moved, slot),
    }
}

fn repository(
    state: &State,
    moved: &Target,
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let held = host_of(state, &moved.project).ok_or(PlanError::SpaceNotLoaded)?;
    let path = Route::scope(&moved.project, &moved.repo);
    let body = json!({"folderId": slot.parent, "order": slot.index});
    Ok(Plan::of(&held.spec.id, vec![Call::patch(path).json(body)]))
}

fn branch(
    state: &State,
    (moved, workspace): (&Target, &str),
    slot: &Slot,
) -> Result<Plan, PlanError> {
    let held: &HostSlot = host_of(state, &moved.project).ok_or(PlanError::SpaceNotLoaded)?;
    let store = &held.store;
    let ws = store
        .workspaces
        .get(workspace)
        .ok_or(PlanError::WorkspaceNotLoaded)?;
    let host = &held.spec.id;
    BranchDrop { store, host, ws }.plan(slot)
}

#[cfg(test)]
mod tests {
    use daemonapi::Frame;
    use serde_json::{Value, json};

    use super::*;
    use crate::test_support::state;

    const FOLDERS: &str = "/v0/projects/p/repos/r/chats/folders/f";

    fn with_a_home_folder() -> State {
        let (mut state, _) = state();
        let folder = json!({"id": "hf", "projectId": "p", "repoId": "", "name": "Notes"});
        let frame = Frame::Entity {
            scope: "projects".into(),
            kind: "folder".into(),
            id: "hf".into(),
            v: 1,
            data: Some(folder),
            gone: false,
        };
        state.apply("local", frame);
        state
    }

    fn sent(
        state: &State,
        (row, target): (&str, &str),
        mode: DropMode,
    ) -> (String, Value) {
        let plan = drop_row(state, (row, target), mode).unwrap();
        let call = &plan.calls[0];
        (call.path.clone(), call.body.clone().unwrap_or_default())
    }

    fn a_thread_dropped_into_a_folder_of_another_workspace_moves_there() {
        let (state, _) = state();
        let (path, body) = sent(&state, ("t", "f"), DropMode::Into);
        assert_eq!(path, "/v0/projects/p/repos/r/chats/t/placement");
        let want = json!({"parentId": "f", "order": 0, "workspaceId": "rd", "label": "Stuff"});
        assert_eq!(body, want);
    }

    fn a_thread_dropped_at_the_repository_root_is_filed_under_its_header() {
        let (state, _) = state();
        let (_, body) = sent(&state, ("t", "f"), DropMode::After);
        let want = json!({"parentId": "rdo", "order": 1, "workspaceId": "rd", "label": "crowbar"});
        assert_eq!(body, want);
    }

    fn a_folder_reordered_where_it_is_sends_only_its_order() {
        let (state, _) = state();
        let (path, body) = sent(&state, ("f", "bo"), DropMode::After);
        assert_eq!(path, FOLDERS);
        assert_eq!(body, json!({"order": 1}));
    }

    fn a_repository_moves_within_the_project_home() {
        let state = with_a_home_folder();
        let repo = "/v0/projects/p/repos/r".to_owned();
        let filed = sent(&state, ("rdo", "hf"), DropMode::Into);
        assert_eq!(filed, (repo.clone(), json!({"folderId": "hf", "order": 0})));
        let before = sent(&state, ("rdo", "hc"), DropMode::Before);
        assert_eq!(before, (repo, json!({"folderId": "", "order": 1})));
    }

    fn a_home_folder_counts_the_repositories_it_is_dropped_among() {
        let state = with_a_home_folder();
        let (path, body) = sent(&state, ("hf", "rdo"), DropMode::After);
        assert_eq!(path, "/v0/projects/p/home/chats/folders/hf");
        assert_eq!(body, json!({"order": 2}));
    }

    const CASES: [fn(); 5] = [
        a_thread_dropped_into_a_folder_of_another_workspace_moves_there,
        a_thread_dropped_at_the_repository_root_is_filed_under_its_header,
        a_folder_reordered_where_it_is_sends_only_its_order,
        a_repository_moves_within_the_project_home,
        a_home_folder_counts_the_repositories_it_is_dropped_among,
    ];

    #[test]
    fn a_drop_is_planned_as_develop_plans_it() {
        for case in CASES {
            case();
        }
    }
}
