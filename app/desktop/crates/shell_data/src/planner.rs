//! Turning a sidebar command into the daemon requests it means.

use serde_json::json;
use sidebar::Command;

use crate::new_id;
use crate::plan::Plan;
use crate::plan_create;
use crate::plan_edit;
use crate::plan_move;
use crate::sidebar_reads::active_project;
use crate::state::State;
use crate::surface::Surface;

pub(crate) fn plan(
    state: &State,
    command: &Command,
) -> Result<Plan, String> {
    let id = new_id::uuid();
    match command {
        Command::NewThread(_)
        | Command::Thread(_)
        | Command::TerminalThread(_)
        | Command::Fork { .. }
        | Command::MakeBranch { .. }
        | Command::CreateFolder(_)
        | Command::CreateSpace { .. } => creation(state, command, &id),
        other => edit(state, other),
    }
}

fn creation(
    state: &State,
    command: &Command,
    id: &str,
) -> Result<Plan, String> {
    match command {
        Command::NewThread(project) => plan_create::home_thread(state, project, id),
        Command::Thread(row) => plan_create::thread(state, row.as_str(), (id, Surface::Default)),
        Command::TerminalThread(row) => {
            plan_create::thread(state, row.as_str(), (id, Surface::Terminal))
        },
        Command::Fork { parent, name } => plan_create::fork(state, (parent.as_str(), name), id),
        Command::MakeBranch { row, name } => plan_create::make_branch(state, (row.as_str(), name)),
        Command::CreateFolder(row) => {
            let active = active_project(state);
            plan_create::folder(
                state,
                row.as_ref().map(sidebar::RowId::as_str),
                active.as_ref(),
            )
        },
        Command::CreateSpace { name, folder, host } => {
            Ok(plan_create::space((name, folder), host, id))
        },
        other => Err(format!("{other:?} is not a data command")),
    }
}

fn edit(
    state: &State,
    command: &Command,
) -> Result<Plan, String> {
    match command {
        Command::DeleteProject(project) => plan_edit::remove_project(state, project),
        Command::Rename { row, name } => plan_edit::rename(state, (row.as_str(), name)),
        Command::CommitRemoval(row) => plan_edit::remove(state, row.as_str()),
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
        other => Err(format!("{other:?} is not a data command")),
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{DropMode, HostId, ProjectId, RowId};

    use super::*;
    use crate::after::After;
    use crate::test_support::state;

    fn plan_of(command: &Command) -> Result<Plan, String> {
        let (state, _) = state();
        plan(&state, command)
    }

    #[test]
    fn a_new_thread_posts_to_the_home_and_opens_when_it_lands() {
        let plan = plan_of(&Command::NewThread(ProjectId::from("p"))).unwrap();
        let call = &plan.calls[0];
        assert_eq!(
            (call.method, call.path.as_str()),
            ("POST", "/v0/projects/p/home/chats")
        );
        let body = call.body.as_ref().unwrap();
        assert_eq!(body["workspaceId"], "hw");
        assert!(plan.needs_provider);
        let id = body["id"].as_str().unwrap();
        assert_eq!(plan.after, After::OpenChat(id.to_owned()));
        assert_eq!(plan.pending.as_ref().map(|row| row.id.as_str()), Some(id));
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
