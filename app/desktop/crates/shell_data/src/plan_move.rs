//! Plans that file a row somewhere else.

use serde_json::json;
use sidebar::DropMode;

use crate::call::Call;
use crate::placement::slot_for;
use crate::plan::Plan;
use crate::routes;
use crate::sidebar_reads::{active_project, host_of, tree};
use crate::slot::Slot;
use crate::state::State;
use crate::target::resolve;
use crate::target_kind::TargetKind;

pub(crate) fn drop_row(
    state: &State,
    (row, target): (&str, &str),
    mode: DropMode,
) -> Result<Plan, String> {
    let project = active_project(state).ok_or("no space is open")?;
    let rows = tree(state, &project).rows;
    let slot = slot_for(&rows, (row, target), mode).ok_or("this row cannot go there")?;
    place(state, row, &slot)
}

fn place(
    state: &State,
    row: &str,
    slot: &Slot,
) -> Result<Plan, String> {
    let moved = resolve(state, row).ok_or("this row is not loaded any more")?;
    let host = host_of(state, &moved.project)
        .map(|held| held.spec.id.clone())
        .ok_or("this space is not loaded yet")?;
    let (project, repo) = (&moved.project, &moved.repo);
    let body = json!({"parentId": slot.parent, "order": slot.index});
    let path = match &moved.kind {
        TargetKind::Folder => format!("{}/{row}", routes::folders(project, repo)),
        TargetKind::Branch {
            workspace,
            locked: true,
            ..
        } => {
            format!(
                "{}/workspaces/{workspace}/placement",
                routes::scope(project, repo)
            )
        },
        TargetKind::Branch { .. } | TargetKind::Chat { .. } => {
            format!(
                "{}/{}/placement",
                routes::chats(project, repo),
                moved.chat()?
            )
        },
        TargetKind::RepoHeader { .. } => {
            return Err("repositories are ordered inside their space, not by dragging".to_owned());
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
