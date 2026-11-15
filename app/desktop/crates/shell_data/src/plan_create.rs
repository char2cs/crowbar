//! Plans that make something new: threads, forks, folders, spaces.

use serde_json::json;
use sidebar::{HostId, ProjectId};

use crate::after::After;
use crate::call::Call;
use crate::pending_row::PendingRow;
use crate::plan::Plan;
use crate::routes;
use crate::scope_items::default_workspace;
use crate::sidebar_reads::host_of;
use crate::state::State;
use crate::surface::Surface;
use crate::target::{Target, resolve};
use crate::target_kind::TargetKind;

const NEW_THREAD: &str = "New thread";
const NEW_WORKSPACE: &str = "New workspace";
const NEW_FOLDER: &str = "New folder";

pub(crate) fn home_thread(
    state: &State,
    project: &ProjectId,
    id: &str,
) -> Result<Plan, String> {
    let slot = host_of(state, project.as_str()).ok_or("this space is not loaded yet")?;
    let home = default_workspace(&slot.store, (project.as_str(), ""))
        .ok_or("this space has no home to run a thread in yet")?;
    let body = chat_body(id, (&home.id, ""), Surface::Default);
    let path = routes::chats(project.as_str(), "");
    let row = pending(id, NEW_THREAD, ("", ""));
    Ok(Plan::of(&slot.spec.id, vec![Call::post(path).json(body)])
        .showing(row)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

pub(crate) fn thread(
    state: &State,
    row: &str,
    (id, surface): (&str, Surface),
) -> Result<Plan, String> {
    let target = resolve(state, row).ok_or("this row is not loaded any more")?;
    let workspace = workspace_for(state, &target)?;
    let parent = parent_chat(&target)?;
    let body = chat_body(id, (&workspace, &parent), surface);
    let path = routes::chats(&target.project, &target.repo);
    let host = host_id(state, &target)?;
    let pending = pending(id, NEW_THREAD, (&target.repo, &parent));
    Ok(Plan::of(&host, vec![Call::post(path).json(body)])
        .showing(pending)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

fn parent_chat(target: &Target) -> Result<String, String> {
    if matches!(target.kind, TargetKind::Branch { .. }) {
        return target.chat().map(str::to_owned);
    }
    Ok(target.id.clone())
}

pub(crate) fn fork(
    state: &State,
    (parent, name): (&str, &str),
    id: &str,
) -> Result<Plan, String> {
    let target = resolve(state, parent).ok_or("this row is not loaded any more")?;
    if target.repo.is_empty() {
        return Err("a project home has no worktree to fork".into());
    }
    let owner = target.chat()?.to_owned();
    let body = json!({"id": id, "provider": null, "parentId": owner, "ownWorktree": true});
    let body = Some(name).filter(|given| !given.is_empty()).map_or_else(
        || body.clone(),
        |given| with_field(body.clone(), "branch", json!(given)),
    );
    let label = Some(name)
        .filter(|given| !given.is_empty())
        .unwrap_or(NEW_WORKSPACE);
    let call = Call::post(routes::chats(&target.project, &target.repo)).json(body);
    let host = host_id(state, &target)?;
    let row = pending(id, label, (&target.repo, &owner));
    Ok(Plan::of(&host, vec![call])
        .showing(row)
        .then(After::OpenChat(id.to_owned()))
        .with_provider())
}

pub(crate) fn folder(
    state: &State,
    row: Option<&str>,
    project: Option<&ProjectId>,
) -> Result<Plan, String> {
    let (project_id, repo, parent, host) =
        row.map_or_else(|| at_top(state, project), |row| beside(state, row))?;
    let call = Call::post(routes::folders(&project_id, &repo))
        .json(json!({"name": NEW_FOLDER, "parentId": parent}));
    Ok(Plan::of(&host, vec![call]))
}

type FolderPlace = (String, String, String, String);

fn beside(
    state: &State,
    row: &str,
) -> Result<FolderPlace, String> {
    let target = resolve(state, row).ok_or("this row is not loaded any more")?;
    let parent = folder_parent(&target)?;
    let host = host_id(state, &target)?;
    Ok((target.project, target.repo, parent, host))
}

fn at_top(
    state: &State,
    project: Option<&ProjectId>,
) -> Result<FolderPlace, String> {
    let project = project.ok_or("no space is open")?;
    let slot = host_of(state, project.as_str()).ok_or("this space is not loaded yet")?;
    Ok((
        project.as_str().to_owned(),
        String::new(),
        String::new(),
        slot.spec.id.clone(),
    ))
}

pub(crate) fn make_branch(
    state: &State,
    (row, name): (&str, &str),
) -> Result<Plan, String> {
    let target = resolve(state, row).ok_or("this row is not loaded any more")?;
    let path = format!(
        "{}/{}/branch-out",
        routes::chats(&target.project, &target.repo),
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
        .json(json!({"name": name, "path": folder}))
        .idempotent(id);
    Plan::of(host.as_str(), vec![call]).then(After::SelectProject(id.to_owned()))
}

pub(crate) fn repo(
    state: &State,
    project: Option<&ProjectId>,
    (path, id): (&str, &str),
) -> Result<Plan, String> {
    let project = project.ok_or("no space is open")?;
    let slot = host_of(state, project.as_str()).ok_or("this space is not loaded yet")?;
    let name = std::path::Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .ok_or("this folder has no name")?;
    let call = Call::post(routes::repos(project.as_str()))
        .json(json!({"name": name, "path": path}))
        .idempotent(id);
    Ok(Plan::of(&slot.spec.id, vec![call]))
}

fn folder_parent(target: &Target) -> Result<String, String> {
    match target.kind {
        TargetKind::Folder => Ok(target.id.clone()),
        TargetKind::Branch { .. } => Ok(target.chat()?.to_owned()),
        _ => Ok(String::new()),
    }
}

fn workspace_for(
    state: &State,
    target: &Target,
) -> Result<String, String> {
    if let Some(workspace) = target.workspace().filter(|workspace| !workspace.is_empty()) {
        return Ok(workspace.to_owned());
    }
    let slot = host_of(state, &target.project).ok_or("this space is not loaded yet")?;
    default_workspace(&slot.store, (&target.project, &target.repo))
        .map(|ws| ws.id.clone())
        .ok_or_else(|| {
            "start a thread from a workspace row, a folder has none to run it in".to_owned()
        })
}

fn host_id(
    state: &State,
    target: &Target,
) -> Result<String, String> {
    host_of(state, &target.project)
        .map(|slot| slot.spec.id.clone())
        .ok_or_else(|| "this space is not loaded yet".to_owned())
}

fn chat_body(
    id: &str,
    (workspace, parent): (&str, &str),
    surface: Surface,
) -> serde_json::Value {
    let body = json!({"id": id, "provider": null, "parentId": parent, "workspaceId": workspace});
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
    use crate::host_spec::HostSpec;
    use crate::surface::Surface;
    use crate::test_support::{state, state_on};

    fn a_thread_under_a_chat_runs_in_its_workspace_and_a_home_thread_in_the_home() {
        let (state, _) = state();
        let plan = thread(&state, "t", ("n", Surface::Default)).unwrap();
        let body = plan.calls[0].body.clone().unwrap();
        assert_eq!(
            (body["workspaceId"].as_str(), body["parentId"].as_str()),
            (Some("b"), Some("t"))
        );
        let home = home_thread(&state, &ProjectId::from("p"), "n").unwrap();
        assert_eq!(home.host, "local");
    }

    fn a_fork_of_a_home_row_is_refused_and_a_folder_with_no_row_needs_a_space() {
        let (state, _) = state();
        assert!(fork(&state, ("hc", ""), "n").is_err());
        assert!(folder(&state, None, None).is_err());
        let root = folder(&state, None, Some(&ProjectId::from("p"))).unwrap();
        assert_eq!(root.calls[0].path, "/v0/projects/p/home/chats/folders");
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
        a_fork_of_a_home_row_is_refused_and_a_folder_with_no_row_needs_a_space,
        make_branch_names_the_branch,
    ];

    #[test]
    fn every_create_plan_is_made_from_the_state() {
        for case in CASES {
            case();
        }
    }
}
