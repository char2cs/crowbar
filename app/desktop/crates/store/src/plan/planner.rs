//! Turning a sidebar command into the daemon requests it means.

use serde_json::json;
use sidebar::Command;

use crate::live::Files;
use crate::live::PlanError;
use crate::live::State;
use crate::live::Surface;
use crate::plan::Plan;
use crate::plan::removal::Removal;
use crate::plan::{
    agent as plan_agent, create as plan_create,
    create::{draft as plan_draft, folder as plan_folder, landing, new_chat as plan_new_chat},
    edit as plan_edit, icon as plan_icon, merge as plan_merge, relocate as plan_move,
};
use crate::reads::active_project;

pub(crate) fn plan(
    (state, files): (&State, &dyn Files),
    command: &Command,
) -> Result<Plan, PlanError> {
    let id = daemonapi::Uuid::random().to_string();
    let failure = crate::plan::failure_title(state, command);
    let plan = match command {
        Command::NewThread(_)
        | Command::NewChat(_)
        | Command::NewTerminalThread(_)
        | Command::Thread(_)
        | Command::TerminalThread(_)
        | Command::Fork { .. }
        | Command::MakeBranch { .. }
        | Command::CreateFolder(_)
        | Command::CreateSpace { .. }
        | Command::CreateSpaceWithIcon { .. } => creation((state, files), command, &id)
            .map(|plan| plan.landing_on(landing::of(state, command))),
        other => edit((state, files), other),
    };
    plan.map(|plan| plan.failing_as(failure))
}

fn drafts(
    state: &State,
    command: &Command,
) -> bool {
    landing::of(state, command) == Surface::Default
}

fn not_data(command: &Command) -> PlanError {
    PlanError::NotADataCommand(format!("{command:?}"))
}

fn thread(
    (state, command): (&State, &Command),
    id: &str,
) -> Result<Plan, PlanError> {
    match command {
        Command::NewThread(project) if drafts(state, command) => {
            plan_draft::home(state, project, id)
        },
        Command::NewThread(project) => {
            plan_create::home_thread(state, project, id, Surface::Default)
        },
        Command::NewChat(project) => plan_new_chat::plan(state, project, id),
        Command::NewTerminalThread(project) => {
            plan_create::home_thread(state, project, id, Surface::Terminal)
        },
        Command::Thread(row) if drafts(state, command) => {
            plan_draft::or_thread(state, row.as_str(), id)
        },
        Command::Thread(row) => plan_create::thread(state, row.as_str(), id, Surface::Default),
        Command::TerminalThread(row) => {
            plan_create::thread(state, row.as_str(), id, Surface::Terminal)
        },
        other => Err(not_data(other)),
    }
}

fn creation(
    (state, files): (&State, &dyn Files),
    command: &Command,
    id: &str,
) -> Result<Plan, PlanError> {
    match command {
        Command::NewThread(_)
        | Command::NewChat(_)
        | Command::NewTerminalThread(_)
        | Command::Thread(_)
        | Command::TerminalThread(_) => thread((state, command), id),
        Command::Fork { parent, name } => plan_create::fork(state, (parent.as_str(), name), id),
        Command::MakeBranch { row, name } => plan_create::make_branch(state, (row.as_str(), name)),
        Command::CreateFolder(row) => {
            let active = active_project(state);
            plan_folder::folder(
                state,
                row.as_ref().map(sidebar::RowId::as_str),
                active.as_ref(),
            )
        },
        Command::CreateSpace { name, folder, host } => {
            Ok(plan_create::space((name, folder), host, id))
        },
        Command::CreateSpaceWithIcon {
            name,
            folder,
            host,
            icon,
        } => {
            let mut plan = plan_create::space((name, folder), host, id);
            plan.calls
                .extend(plan_icon::calls(files, &plan_icon::space_route(id), icon)?);
            Ok(plan)
        },
        other => Err(not_data(other)),
    }
}

fn edit(
    (state, files): (&State, &dyn Files),
    command: &Command,
) -> Result<Plan, PlanError> {
    match command {
        Command::DeleteProject(project) => plan_edit::remove_project(state, project),
        Command::Rename { row, name } => plan_edit::rename(state, (row.as_str(), name)),
        Command::CommitRemoval(row) => plan_edit::remove(state, Removal::KeepWork(row.clone())),
        Command::RemoveDiscardingWork(row) => {
            plan_edit::remove(state, Removal::DiscardWork(row.clone()))
        },
        Command::Lock(row) => {
            plan_edit::verb(state, row.as_str(), ("lock", Some(json!({"locked": true}))))
        },
        Command::Unlock(row) => plan_edit::verb(
            state,
            row.as_str(),
            ("lock", Some(json!({"locked": false}))),
        ),
        Command::Detach(row) => plan_edit::verb(state, row.as_str(), ("detach-holder", None)),
        Command::Retry(row) => plan_edit::verb(state, row.as_str(), ("retry-provision", None)),
        Command::SetSpaceIcon { project, icon } => plan_icon::space((state, files), project, icon),
        Command::SetRepoIcon { row, icon } => plan_icon::repo((state, files), (row.as_str(), icon)),
        Command::UseGithubAvatar(row) => plan_icon::github(state, row.as_str()),
        Command::SetMergeStrategy(strategy) => plan_merge::set_strategy(state, *strategy),
        Command::Merge { after } => plan_merge::merge(state, *after),
        Command::StopAndMerge => plan_merge::stop_and_merge(state),
        Command::CommitChanges { message, paths } => plan_merge::commit(state, (message, paths)),
        Command::KillAgent(row) => plan_agent::kill(state, row),
        Command::CancelMove(row) => plan_edit::verb(state, row.as_str(), ("move/cancel", None)),
        Command::Dismiss(row) => plan_edit::dismiss(state, row.as_str()),
        Command::MoveTo { row, target } => plan_move::drop_row(
            state,
            (row.as_str(), target.as_str()),
            sidebar::DropMode::Into,
        ),
        Command::Drop { row, target, mode } => {
            plan_move::drop_row(state, (row.as_str(), target.as_str()), *mode)
        },
        other => Err(not_data(other)),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{DropMode, HostId, ProjectId, RowId};

    use super::*;
    use crate::plan::after::After;
    use crate::test_support::{FakeFiles, state};

    fn plan_of(command: &Command) -> Result<Plan, PlanError> {
        let (state, _) = state();
        plan((&state, &FakeFiles::default()), command)
    }

    #[test]
    fn a_plan_carries_the_title_its_failure_is_shown_under() {
        let lock = plan_of(&Command::Lock(RowId::from("b"))).unwrap();
        assert_eq!(lock.failure.as_deref(), Some("Failed to update lock"));
    }

    #[test]
    fn a_plain_new_thread_opens_a_draft_and_sends_nothing() {
        for command in [
            Command::NewThread(ProjectId::from("p")),
            Command::Thread(RowId::from("bo")),
        ] {
            let plan = plan_of(&command).unwrap();
            assert!(plan.calls.is_empty(), "{command:?}: {:?}", plan.calls);
            assert!(
                matches!(plan.after, After::OpenDraft(_)),
                "{command:?}: {:?}",
                plan.after
            );
        }
    }

    #[test]
    fn the_new_chat_chord_makes_its_chat_in_the_focused_chats_workspace() {
        let (mut state, _) = state();
        state.panels.follow(Some("bo".into()));
        let plan = plan(
            (&state, &FakeFiles::default()),
            &Command::NewChat(ProjectId::from("p")),
        )
        .unwrap();
        let call = &plan.calls[0];
        assert_eq!(
            (call.method, call.path.as_str()),
            ("POST", "/v0/projects/p/repos/r/chats")
        );
        assert_eq!(call.body.as_ref().unwrap()["workspaceId"], "b");
        assert!(plan.needs_provider);
    }

    #[test]
    fn a_new_thread_on_the_terminal_by_default_is_created_at_once() {
        let (state, _) = state();
        state.settings.install(|key| {
            (key.as_str() == "chatIsDefaultPresentation").then_some(settings::Value::Bool(false))
        });
        let plan = plan(
            (&state, &FakeFiles::default()),
            &Command::NewThread(ProjectId::from("p")),
        )
        .unwrap();
        let call = &plan.calls[0];
        assert_eq!(
            (call.method, call.path.as_str()),
            ("POST", "/v0/projects/p/home/chats")
        );
        let id = call.body.as_ref().unwrap()["id"].as_str().unwrap();
        assert_eq!(plan.after, After::OpenChat(id.to_owned()));
    }

    fn command(
        verb: &str,
        row: &str,
    ) -> Command {
        let row = RowId::from(row);
        let name = "x".to_owned();
        match verb {
            "rename" => Command::Rename { row, name },
            "remove" => Command::CommitRemoval(row),
            "lock" => Command::Lock(row),
            "detach" => Command::Detach(row),
            "retry" => Command::Retry(row),
            "cancel" => Command::CancelMove(row),
            "branch" => Command::MakeBranch { row, name },
            _ => Command::DeleteProject(ProjectId::from("p")),
        }
    }

    type Route = (&'static str, &'static str, &'static str, &'static str);

    const CASES: [Route; 11] = [
        ("rename", "t", "POST", "CHATS/t/rename"),
        ("rename", "f", "PATCH", "CHATS/folders/f"),
        ("rename", "rdo", "PATCH", "/v0/projects/p/repos/r"),
        ("remove", "t", "DELETE", "CHATS/t"),
        ("remove", "bo", "DELETE", "CHATS/bo"),
        ("lock", "bo", "POST", "CHATS/bo/lock"),
        ("detach", "bo", "POST", "CHATS/bo/detach-holder"),
        ("retry", "bo", "POST", "CHATS/bo/retry-provision"),
        ("cancel", "t", "POST", "CHATS/t/move/cancel"),
        ("branch", "t", "POST", "CHATS/t/branch-out"),
        ("delete", "p", "DELETE", "/v0/projects/p"),
    ];

    #[test]
    fn each_row_command_names_its_daemon_route() {
        let chats = "/v0/projects/p/repos/r/chats";
        for (verb, row, method, path) in CASES {
            let plan = plan_of(&command(verb, row)).unwrap();
            let call = &plan.calls[0];
            assert_eq!(
                (call.method, call.path.clone()),
                (method, path.replace("CHATS", chats)),
                "{verb} {row}"
            );
        }
    }

    #[test]
    fn forks_threads_and_folders_carry_their_parents() {
        let plan = plan_of(&Command::Fork {
            parent: RowId::from("bo"),
            name: "topic".into(),
        })
        .unwrap();
        let body = plan.calls[0].body.clone().unwrap();
        assert_eq!(
            (
                body["parentId"].as_str(),
                body["ownWorktree"].as_bool(),
                body["branch"].as_str()
            ),
            (Some("bo"), Some(true), Some("topic"))
        );
        let terminal = plan_of(&Command::TerminalThread(RowId::from("bo"))).unwrap();
        assert_eq!(
            terminal.calls[0].body.as_ref().unwrap()["surface"],
            "terminal"
        );
        let folder = plan_of(&Command::CreateFolder(Some(RowId::from("f")))).unwrap();
        assert_eq!(folder.calls[0].path, "/v0/projects/p/repos/r/chats/folders");
        assert_eq!(folder.calls[0].body.as_ref().unwrap()["parentId"], "f");
    }

    #[test]
    fn a_space_is_created_under_an_idempotency_key_and_selected_after() {
        let command = Command::CreateSpace {
            name: "N".into(),
            folder: "/x".into(),
            host: HostId::from("local"),
        };
        let plan = plan_of(&command).unwrap();
        let key = plan.calls[0].headers[0].1.clone();
        assert_eq!(plan.after, After::SelectProject(key));
    }

    #[test]
    fn moves_resolve_to_a_placement_and_unknown_rows_are_refused() {
        let moved = plan_of(&Command::Drop {
            row: RowId::from("t"),
            target: RowId::from("f"),
            mode: DropMode::Into,
        })
        .unwrap();
        assert_eq!(moved.calls[0].method, "PATCH");
        assert!(plan_of(&Command::Lock(RowId::from("ghost"))).is_err());
        assert!(plan_of(&Command::OpenSettings).is_err());
    }
}
