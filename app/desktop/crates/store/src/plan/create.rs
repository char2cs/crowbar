//! Plans that make something new: threads, forks, folders, spaces.

pub(crate) mod folder;
pub(crate) mod landing;

use daemonapi::Call;
use serde_json::json;
use sidebar::{HostId, ProjectId};

use crate::live::PendingRow;
use crate::live::PlanError;
use crate::live::State;
use crate::live::Surface;
use crate::plan::Plan;
use crate::plan::after::After;
use crate::reads::host_of;
use crate::rows::default_workspace;
use crate::target::Kind;
use crate::target::{Target, resolve};

const NEW_THREAD: &str = "New thread";
const NEW_WORKSPACE: &str = "New workspace";
pub(crate) const PARENT_ID: &str = "parentId";
const NAME: &str = "name";

pub(crate) fn home_thread(
    state: &State,
    project: &ProjectId,
    id: &str,
    surface: Surface,
) -> Result<Plan, PlanError> {
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let home = default_workspace(&slot.store, (project.as_str(), "")).ok_or(PlanError::NoHome)?;
    let body = chat_body(id, (&home.id, ""), surface);
    let path = daemonapi::Route::chats(project.as_str(), "");
    let row = pending(id, NEW_THREAD, ("", ""));
    Ok(Plan::of(&slot.spec.id, vec![Call::post(path).json(body)])
        .showing(row)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

pub(crate) fn thread(
    state: &State,
    row: &str,
    id: &str,
    surface: Surface,
) -> Result<Plan, PlanError> {
    let target = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    let workspace = workspace_for(state, &target)?;
    let parent = parent_chat(&target)?;
    let body = chat_body(id, (&workspace, &parent), surface);
    let path = daemonapi::Route::chats(&target.project, &target.repo);
    let host = host_id(state, &target)?;
    let pending = pending(id, NEW_THREAD, (&target.repo, &parent));
    Ok(Plan::of(&host, vec![Call::post(path).json(body)])
        .showing(pending)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

fn parent_chat(target: &Target) -> Result<String, PlanError> {
    if matches!(target.kind, Kind::Branch { .. }) {
        return target.chat().map(str::to_owned);
    }
    Ok(target.id.clone())
}

pub(crate) fn fork(
    state: &State,
    (parent, name): (&str, &str),
    id: &str,
) -> Result<Plan, PlanError> {
    let target = resolve(state, parent).ok_or(PlanError::RowNotLoaded)?;
    if target.repo.is_empty() {
        return Err(PlanError::HomeHasNoWorktree);
    }
    let owner = target.chat()?.to_owned();
    let body = json!({"id": id, "provider": null, PARENT_ID: owner, "ownWorktree": true});
    let body = Some(name).filter(|given| !given.is_empty()).map_or_else(
        || body.clone(),
        |given| with_field(body.clone(), "branch", json!(given)),
    );
    let label = Some(name)
        .filter(|given| !given.is_empty())
        .unwrap_or(NEW_WORKSPACE);
    let call = Call::post(daemonapi::Route::chats(&target.project, &target.repo)).json(body);
    let host = host_id(state, &target)?;
    let row = pending(id, label, (&target.repo, &owner));
    Ok(Plan::of(&host, vec![call])
        .showing(row)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

pub(crate) fn make_branch(
    state: &State,
    (row, name): (&str, &str),
) -> Result<Plan, PlanError> {
    let target = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    let path = format!(
        "{}/{}/branch-out",
        daemonapi::Route::chats(&target.project, &target.repo),
        target.chat()?
    );
    Ok(Plan::of(
        &host_id(state, &target)?,
        vec![Call::post(path).json(json!({"branch": name}))],
    ))
}

pub(crate) fn space(
    (name, folder): (&str, &str),
    host: &HostId,
    id: &str,
) -> Plan {
    let call = Call::post("/v0/projects")
        .json(json!({NAME: name, "path": folder}))
        .idempotent(id);
    Plan::of(host.as_str(), vec![call]).then(After::SelectProject(id.to_owned()))
}

pub(crate) fn repo(
    state: &State,
    project: Option<&ProjectId>,
    (path, id): (&str, &str),
) -> Result<Plan, PlanError> {
    let project = project.ok_or(PlanError::NoSpaceOpen)?;
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let name = std::path::Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .ok_or(PlanError::FolderHasNoName)?;
    let call = Call::post(daemonapi::Route::repos(project.as_str()))
        .json(json!({NAME: name, "path": path}))
        .idempotent(id);
    Ok(Plan::of(&slot.spec.id, vec![call]))
}

fn workspace_for(
    state: &State,
    target: &Target,
) -> Result<String, PlanError> {
    if let Some(workspace) = target.workspace().filter(|workspace| !workspace.is_empty()) {
        return Ok(workspace.to_owned());
    }
    let slot = host_of(state, &target.project).ok_or(PlanError::SpaceNotLoaded)?;
    default_workspace(&slot.store, (&target.project, &target.repo))
        .map(|ws| ws.id.clone())
        .ok_or(PlanError::NoWorkspaceToRunIn)
}

pub(crate) fn host_id(
    state: &State,
    target: &Target,
) -> Result<String, PlanError> {
    host_of(state, &target.project)
        .map(|slot| slot.spec.id.clone())
        .ok_or(PlanError::SpaceNotLoaded)
}

fn chat_body(
    id: &str,
    (workspace, parent): (&str, &str),
    surface: Surface,
) -> serde_json::Value {
    let body = json!({"id": id, "provider": null, PARENT_ID: parent, "workspaceId": workspace});
    if surface == Surface::Terminal {
        return with_field(body, "surface", json!("terminal"));
    }
    body
}

fn with_field(
    mut body: serde_json::Value,
    key: &str,
    value: serde_json::Value,
) -> serde_json::Value {
    if let serde_json::Value::Object(map) = &mut body {
        map.insert(key.to_owned(), value);
    }
    body
}

fn pending(
    id: &str,
    label: &str,
    (scope, parent): (&str, &str),
) -> PendingRow {
    PendingRow {
        id: id.to_owned(),
        label: label.to_owned(),
        scope: scope.to_owned(),
        parent: parent.to_owned(),
        failure: None,
        operation: None,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::ProjectId;

    use super::*;
    use crate::live::HostSpec;
    use crate::live::Surface;
    use crate::test_support::{state, state_on};

    fn a_thread_under_a_chat_runs_in_its_workspace_and_a_home_thread_in_the_home() {
        let (state, _) = state();
        let plan = thread(&state, "t", "n", Surface::Default).unwrap();
        let body = plan.calls[0].body.clone().unwrap();
        assert_eq!(
            (body["workspaceId"].as_str(), body["parentId"].as_str()),
            (Some("b"), Some("t"))
        );
        let home = home_thread(&state, &ProjectId::from("p"), "n", Surface::Default).unwrap();
        assert_eq!(home.host, "local");
        let terminal = home_thread(&state, &ProjectId::from("p"), "n", Surface::Terminal);
        let body = terminal.unwrap().calls[0].body.clone().unwrap();
        assert_eq!(body["surface"].as_str(), Some("terminal"));
    }

    fn a_fork_of_a_home_row_is_refused() {
        let (state, _) = state();
        assert!(fork(&state, ("hc", ""), "n").is_err());
    }

    fn make_branch_names_the_branch() {
        let (state, _) = state();
        let plan = make_branch(&state, ("t", "topic")).unwrap();
        assert_eq!(plan.calls[0].body.clone().unwrap()["branch"], "topic");
    }

    fn a_repo_is_added_on_the_host_of_its_space_local_or_remote() {
        let remote = HostSpec {
            id: "box".into(),
            name: "Box".into(),
            endpoint: Some("box.lan:7000".into()),
        };
        for spec in [None, Some(remote)] {
            let (state, _) = spec.map_or_else(state, state_on);
            let host = state.hosts[0].spec.id.clone();
            let project = ProjectId::from("p");
            let plan = repo(&state, Some(&project), ("/srv/app/", "k1")).unwrap();
            let call = &plan.calls[0];
            assert_eq!(plan.host, host);
            assert_eq!(call.path, "/v0/projects/p/repos");
            assert_eq!(call.body.clone().unwrap()["name"], "app");
            assert_eq!(call.body.clone().unwrap()["path"], "/srv/app/");
        }
    }

    fn a_repo_needs_an_open_space_and_a_named_folder() {
        let (state, _) = state();
        assert!(repo(&state, None, ("/srv/app", "k")).is_err());
        assert!(repo(&state, Some(&ProjectId::from("p")), ("/", "k")).is_err());
        assert!(repo(&state, Some(&ProjectId::from("gone")), ("/srv/app", "k")).is_err());
    }

    const CASES: [fn(); 5] = [
        a_repo_is_added_on_the_host_of_its_space_local_or_remote,
        a_repo_needs_an_open_space_and_a_named_folder,
        a_thread_under_a_chat_runs_in_its_workspace_and_a_home_thread_in_the_home,
        a_fork_of_a_home_row_is_refused,
        make_branch_names_the_branch,
    ];

    #[test]
    fn every_create_plan_is_made_from_the_state() {
        for case in CASES {
            case();
        }
    }
}
