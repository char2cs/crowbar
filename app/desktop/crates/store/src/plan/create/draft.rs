//! New threads that open as drafts, as develop's draft pane: no chat exists until the first send.

use sidebar::ProjectId;

use crate::live::DraftThread;
use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::after::After;
use crate::plan::create::{host_id, parent_chat, thread, workspace_for};
use crate::reads::host_of;
use crate::rows::default_workspace;
use crate::rows::target::{Kind, resolve};

pub(crate) fn home(
    state: &State,
    project: &ProjectId,
    id: &str,
) -> Result<Plan, PlanError> {
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let home = default_workspace(&slot.store, (project.as_str(), "")).ok_or(PlanError::NoHome)?;
    let draft = DraftThread {
        id: DraftThread::minted(id),
        project: project.as_str().to_owned(),
        repo: String::new(),
        workspace: home.id.clone(),
        parent: String::new(),
    };
    Ok(Plan::of(&slot.spec.id, Vec::new()).then(After::OpenDraft(draft)))
}

pub(crate) fn or_thread(
    state: &State,
    row: &str,
    id: &str,
) -> Result<Plan, PlanError> {
    let target = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    if !matches!(target.kind, Kind::Branch { .. } | Kind::RepoHeader { .. }) {
        return thread(state, row, id);
    }
    let draft = DraftThread {
        id: DraftThread::minted(id),
        project: target.project.clone(),
        repo: target.repo.clone(),
        workspace: workspace_for(state, &target)?,
        parent: parent_chat(&target)?,
    };
    Ok(Plan::of(&host_id(state, &target)?, Vec::new()).then(After::OpenDraft(draft)))
}

#[cfg(test)]
mod tests {
    use sidebar::ProjectId;

    use super::*;
    use crate::test_support::state;

    fn drafted(plan: &Plan) -> DraftThread {
        assert!(plan.calls.is_empty(), "nothing is sent: {:?}", plan.calls);
        assert!(
            plan.pending.is_none() && !plan.needs_provider,
            "no row, no agent"
        );
        let After::OpenDraft(draft) = plan.after.clone() else {
            panic!("a draft opens: {:?}", plan.after);
        };
        assert_eq!(draft.id, "draft-u1");
        draft
    }

    #[test]
    fn a_home_draft_names_the_home_workspace_and_the_panel_root() {
        let (state, _) = state();
        let draft = drafted(&home(&state, &ProjectId::from("p"), "u1").unwrap());
        assert_eq!(
            (draft.project, draft.repo, draft.workspace, draft.parent),
            ("p".into(), String::new(), "hw".into(), String::new())
        );
    }

    #[test]
    fn a_thread_on_a_branch_drafts_under_its_chat_and_one_under_a_chat_is_created() {
        let (state, _) = state();
        let draft = drafted(&or_thread(&state, "bo", "u1").unwrap());
        assert_eq!(
            (draft.repo, draft.workspace, draft.parent),
            ("r".into(), "b".into(), "bo".into())
        );
        let nested = or_thread(&state, "t", "u1").unwrap();
        assert_eq!(nested.calls[0].body.as_ref().unwrap()["parentId"], "t");
    }
}
