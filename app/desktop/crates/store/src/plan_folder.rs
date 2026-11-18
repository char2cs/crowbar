//! The plan that makes a folder in the chat tree: at the top of a space or beside a row.

use serde_json::json;
use sidebar::ProjectId;

use crate::call::Call;
use crate::plan::Plan;
use crate::plan_create::{PARENT_ID, host_id};
use crate::plan_error::PlanError;
use crate::routes;
use crate::sidebar_reads::host_of;
use crate::state::State;
use crate::target::{Target, resolve};
use crate::target_kind::TargetKind;

const NEW_FOLDER: &str = "New folder";
const NAME: &str = "name";

pub(crate) fn folder(
    state: &State,
    row: Option<&str>,
    project: Option<&ProjectId>,
) -> Result<Plan, PlanError> {
    let (project_id, repo, parent, host) =
        row.map_or_else(|| at_top(state, project), |row| beside(state, row))?;
    let call = Call::post(routes::folders(&project_id, &repo))
        .json(json!({NAME: NEW_FOLDER, PARENT_ID: parent}));
    Ok(Plan::of(&host, vec![call]))
}

type FolderPlace = (String, String, String, String);

fn beside(
    state: &State,
    row: &str,
) -> Result<FolderPlace, PlanError> {
    let target = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    let parent = folder_parent(&target)?;
    let host = host_id(state, &target)?;
    Ok((target.project, target.repo, parent, host))
}

fn at_top(
    state: &State,
    project: Option<&ProjectId>,
) -> Result<FolderPlace, PlanError> {
    let project = project.ok_or(PlanError::NoSpaceOpen)?;
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    Ok((
        project.as_str().to_owned(),
        String::new(),
        String::new(),
        slot.spec.id.clone(),
    ))
}

fn folder_parent(target: &Target) -> Result<String, PlanError> {
    match target.kind {
        TargetKind::Folder => Ok(target.id.clone()),
        TargetKind::Branch { .. } => Ok(target.chat()?.to_owned()),
        _ => Ok(String::new()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_folder_with_no_row_needs_a_space_and_beside_a_row_it_carries_its_parent() {
        let (state, _) = state();
        assert!(folder(&state, None, None).is_err());
        let root = folder(&state, None, Some(&ProjectId::from("p"))).unwrap();
        assert_eq!(root.calls[0].path, "/v0/projects/p/home/chats/folders");
        let beside = folder(&state, Some("f"), None).unwrap();
        assert_eq!(beside.calls[0].body.as_ref().unwrap()["parentId"], "f");
        assert!(folder(&state, Some("ghost"), None).is_err());
    }
}
