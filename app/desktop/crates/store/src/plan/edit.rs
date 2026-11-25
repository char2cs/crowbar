//! Plans that change or remove something that exists.

use daemonapi::Call;
use serde_json::json;
use sidebar::ProjectId;

use crate::live::HostSlot;
use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::after::After;
use crate::plan::removal::{Removal, Taken};
use crate::reads::host_of;
use crate::rows::pending as pending_rows;
use crate::rows::target::Kind;
use crate::rows::target::{Target, resolve};

pub(crate) fn locate(
    state: &State,
    row: &str,
) -> Result<(Target, String), PlanError> {
    let target = resolve(state, row).ok_or(PlanError::RowNotLoaded)?;
    let host = host_of(state, &target.project)
        .map(|slot| slot.spec.id.clone())
        .ok_or(PlanError::SpaceNotLoaded)?;
    Ok((target, host))
}

pub(crate) fn rename(
    state: &State,
    (row, name): (&str, &str),
) -> Result<Plan, PlanError> {
    let (target, host) = locate(state, row)?;
    let (project, repo) = (&target.project, &target.repo);
    let call = match &target.kind {
        Kind::Folder => Call::patch(format!(
            "{}/{row}",
            daemonapi::Route::folders(project, repo)
        ))
        .json(json!({"name": name})),
        Kind::RepoHeader { .. } => {
            Call::patch(daemonapi::Route::scope(project, repo)).json(json!({"name": name}))
        },
        Kind::Branch { locked: true, .. } => Call::patch(format!(
            "{}/{}/branch",
            daemonapi::Route::chats(project, repo),
            target.chat()?
        ))
        .json(json!({"branch": name})),
        _ => Call::post(format!(
            "{}/{}/rename",
            daemonapi::Route::chats(project, repo),
            target.chat()?
        ))
        .json(json!({"title": name})),
    };
    Ok(Plan::of(&host, vec![call]))
}

pub(crate) fn remove(
    state: &State,
    removal: Removal,
) -> Result<Plan, PlanError> {
    let row = removal.row().as_str();
    let (target, host) = locate(state, row)?;
    let (project, repo) = (&target.project, &target.repo);
    let (call, after) = match &target.kind {
        Kind::Folder => (
            Call::delete(format!(
                "{}/{row}",
                daemonapi::Route::folders(project, repo)
            )),
            After::Nothing,
        ),
        Kind::RepoHeader { .. } => (
            Call::delete(daemonapi::Route::scope(project, repo))
                .idempotent(&daemonapi::Uuid::random().to_string()),
            After::Nothing,
        ),
        _ => chat_delete(state, &target)?,
    };
    let call = if_consented(call, &removal);
    Ok(Plan::of(&host, vec![call]).removing(removal).then(after))
}

fn chat_delete(
    state: &State,
    target: &Target,
) -> Result<(Call, After), PlanError> {
    let chat = target.chat()?;
    let route = daemonapi::Route::chats(&target.project, &target.repo);
    let taken = Taken::before(state, (&target.project, chat));
    Ok((
        Call::delete(format!("{route}/{chat}")),
        After::AnnounceTaken(taken),
    ))
}

fn if_consented(
    call: Call,
    removal: &Removal,
) -> Call {
    if !removal.discards_work() {
        return call;
    }
    call.discarding_work()
}

pub(crate) fn remove_project(
    state: &State,
    project: &ProjectId,
) -> Result<Plan, PlanError> {
    let slot = host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let call = Call::delete(daemonapi::Route::project(project.as_str()))
        .idempotent(&daemonapi::Uuid::random().to_string());
    Ok(Plan::of(&slot.spec.id, vec![call]))
}

pub(crate) fn verb(
    state: &State,
    row: &str,
    (verb, body): (&str, Option<serde_json::Value>),
) -> Result<Plan, PlanError> {
    let (target, host) = locate(state, row)?;
    let path = format!(
        "{}/{}/{verb}",
        daemonapi::Route::chats(&target.project, &target.repo),
        target.chat()?
    );
    let mut call = Call::post(path);
    if let Some(body) = body {
        call = call.json(body);
    }
    if matches!(verb, "detach-holder" | "retry-provision") {
        call = call.idempotent(&daemonapi::Uuid::random().to_string());
    }
    Ok(Plan::of(&host, vec![call]))
}

pub(crate) fn dismiss(
    state: &State,
    row: &str,
) -> Result<Plan, PlanError> {
    if state.local_rows.iter().any(|held| held.id == row) {
        let mut plan = Plan::of("", Vec::new());
        plan.dropping = Some(row.to_owned());
        return Ok(plan);
    }
    state
        .hosts
        .iter()
        .find_map(|slot| dismissal(slot, row))
        .ok_or(PlanError::NothingToDismiss)
}

fn dismissal(
    slot: &HostSlot,
    row: &str,
) -> Option<Plan> {
    let operation = pending_rows::of(&slot.store)
        .into_iter()
        .find(|held| held.id == row)
        .and_then(|held| held.operation)?;
    Some(Plan::of(
        &slot.spec.id,
        vec![Call::delete(format!("/v0/operations/{operation}"))],
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::live::PendingRow;
    use crate::test_support::state;
    use sidebar::RowId;

    fn discards(
        state: &State,
        removal: Removal,
    ) -> bool {
        let plan = remove(state, removal).unwrap();
        plan.calls[0]
            .headers
            .iter()
            .any(|(name, _)| name == "crowbar-discard-work")
    }

    fn a_locked_branch_renames_its_git_branch_and_only_consent_discards_work() {
        let (mut state, _) = state();
        let plan = rename(&state, ("t", "new")).unwrap();
        assert!(plan.calls[0].path.ends_with("/t/rename"));
        let first = discards(&state, Removal::KeepWork(RowId::from("t")));
        assert!(!first, "the first delete keeps work at risk");
        let consented = discards(&state, Removal::DiscardWork(RowId::from("t")));
        assert!(consented, "the confirmed delete discards it");
        let folder = remove(&state, Removal::KeepWork(RowId::from("f"))).unwrap();
        assert_eq!(folder.calls[0].headers.len(), 0);
        state.hosts[0].store.workspaces.put("b", 2, serde_json::from_value(serde_json::json!({"id": "b", "projectId": "p", "repoId": "r", "status": "locked", "branch": "feat", "owningChatId": "bo"})).unwrap());
        let locked = rename(&state, ("bo", "other")).unwrap();
        assert!(locked.calls[0].path.ends_with("/bo/branch"));
    }

    fn dismissing_a_local_row_sends_nothing_and_an_unknown_one_is_an_error() {
        let (mut state, _) = state();
        state.add_local(PendingRow {
            id: "x".into(),
            label: "New".into(),
            scope: String::new(),
            parent: String::new(),
            failure: None,
            operation: None,
        });
        let plan = dismiss(&state, "x").unwrap();
        assert_eq!(plan.calls.len(), 0);
        assert_eq!(plan.dropping.as_deref(), Some("x"));
        assert!(dismiss(&state, "other").is_err());
    }

    fn verbs_post_to_the_owning_chat() {
        let (state, _) = state();
        let plan = verb(
            &state,
            "bo",
            ("lock", Some(serde_json::json!({"locked": true}))),
        )
        .unwrap();
        assert!(plan.calls[0].path.ends_with("/bo/lock"));
        let keyed = verb(&state, "bo", ("retry-provision", None)).unwrap();
        assert_ne!(keyed.calls[0].headers.len(), 0);
    }

    const CASES: [fn(); 3] = [
        a_locked_branch_renames_its_git_branch_and_only_consent_discards_work,
        dismissing_a_local_row_sends_nothing_and_an_unknown_one_is_an_error,
        verbs_post_to_the_owning_chat,
    ];

    #[test]
    fn every_edit_plan_is_made_from_the_state() {
        for case in CASES {
            case();
        }
    }
}
