use daemonapi::Call;
use sidebar::ProjectId;

use crate::live::PlanError;
use crate::live::State;
use crate::live::Surface;
use crate::plan::Plan;
use crate::plan::after::After;
use crate::plan::create::chat_body;
use crate::reads::host_of;
use crate::rows::default_workspace;

pub(crate) fn plan(
    state: &State,
    project: &ProjectId,
    id: &str,
) -> Result<Plan, PlanError> {
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let (workspace, repo) = match focused_workspace(state, project.as_str()) {
        Some(found) => found,
        None => home_of(state, project.as_str())?,
    };
    let body = chat_body(id, (&workspace, ""), Surface::Default);
    let path = daemonapi::Route::chats(project.as_str(), &repo);
    Ok(Plan::of(&slot.spec.id, vec![Call::post(path).json(body)])
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

fn focused_workspace(
    state: &State,
    project: &str,
) -> Option<(String, String)> {
    let chat = state.panels.chat.as_deref()?;
    let slot = host_of(state, project)?;
    let record = slot.store.chats.get(chat)?;
    let ours = record.project_id == project && !record.workspace_id.is_empty();
    ours.then(|| (record.workspace_id.clone(), record.repo_id.clone()))
}

fn home_of(
    state: &State,
    project: &str,
) -> Result<(String, String), PlanError> {
    let slot = host_of(state, project).ok_or(PlanError::SpaceNotLoaded)?;
    let home = default_workspace(&slot.store, (project, "")).ok_or(PlanError::NoHome)?;
    Ok((home.id.clone(), String::new()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    fn posted(plan: &Plan) -> (String, String) {
        let call = &plan.calls[0];
        let body = call.body.as_ref().unwrap();
        assert_eq!(body["parentId"], "", "unplaced, at the panel root");
        assert_eq!(
            plan.after,
            After::OpenChat(body["id"].as_str().unwrap().into())
        );
        assert!(plan.pending.is_none(), "no operation row, as develop");
        (
            call.path.as_str().to_owned(),
            body["workspaceId"].as_str().unwrap().into(),
        )
    }

    #[test]
    fn the_chord_makes_its_chat_in_the_focused_chats_workspace_else_the_home() {
        let (mut state, _) = state();
        let home = posted(&plan(&state, &ProjectId::from("p"), "n1").unwrap());
        assert_eq!(home, ("/v0/projects/p/home/chats".into(), "hw".into()));
        state.panels.follow(Some("bo".into()));
        let focused = posted(&plan(&state, &ProjectId::from("p"), "n2").unwrap());
        assert_eq!(focused, ("/v0/projects/p/repos/r/chats".into(), "b".into()));
    }
}
