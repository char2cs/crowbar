//! The sidebar's write side: every command that has a daemon route.

mod copy_name;
mod env_template_file;
mod file_changes;
mod file_collapse;
mod file_commands;
mod file_done;
mod file_error;
mod file_listing;
mod file_session;
mod file_upload;
mod new_entry;

pub use env_template_file::{Overwrite, TemplateOutcome};
pub use file_done::FileDone;
pub use file_error::FileError;
pub use new_entry::NewEntry;

use std::sync::Arc;

use sidebar::{Command, Error, Sink};
use tokio::runtime::Handle;

use crate::live::Files;
use crate::live::Fleet;
use crate::live::Hub;
use crate::live::OutcomeSink;
use crate::live::PlanError;
use crate::plan::Plan;
use crate::reads::{active_project, host_of};
use crate::{
    live::panel_loader,
    plan::{agent as plan_agent, create as plan_create, planner, runner},
};
use daemonapi::new_id;

/// Plans each command from the current entities and runs it on the runtime; the stream shows the
/// result. Commands that are the shell's own (dialogs, opening a pane) are refused here.
#[derive(Clone)]
pub struct SidebarCommands {
    pub(crate) hub: Arc<Hub>,
    pub(crate) handle: Handle,
    pub(crate) outcome: OutcomeSink,
    pub(crate) fleet: Arc<Fleet>,
    pub(crate) files: Arc<dyn Files>,
}

impl std::fmt::Debug for SidebarCommands {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SidebarCommands")
            .finish_non_exhaustive()
    }
}

impl SidebarCommands {
    fn select_host(
        &self,
        host: &str,
    ) {
        self.hub.edit(|state| {
            state.selected_host = Some(host.to_owned());
            state.active_project = None;
        });
    }

    fn retry(
        &self,
        project: &str,
    ) {
        let host = self
            .hub
            .read(|state| host_of(state, project).map(|slot| slot.spec.id.clone()));
        self.fleet.resubscribe(host.as_deref());
    }

    fn toggle(
        &self,
        path: &str,
    ) {
        let (open, listed, chat, client) = self.hub.edit(|state| {
            let open = state.panels.toggle(path);
            let chat = state.panels.chat.clone();
            let client = chat.as_deref().and_then(|chat| state.client_of_chat(chat));
            (open, state.panels.listings.contains_key(path), chat, client)
        });
        let (Some(chat), Some(client)) = (chat, client) else {
            return;
        };
        if !open || listed {
            return;
        }
        let (hub, path) = (Arc::clone(&self.hub), path.to_owned());
        self.handle.spawn(async move {
            panel_loader::load_directory(&hub, &client, (&chat, &path)).await;
        });
    }

    /// Adds the repository at `path`, a folder of the open space's own host, to that space.
    ///
    /// # Errors
    /// [`Error::Refused`] when no space is open, its host is not loaded, or the folder has no name.
    pub fn import_repo(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        let plan = self.hub.read(|state| {
            let project = active_project(state);
            plan_create::repo(state, project.as_ref(), (path, &new_id::uuid()))
        });
        self.run(plan)
    }

    /// Starts the runner of `chat` again; a live chat answers with its runner and nothing changes.
    ///
    /// # Errors
    /// [`Error::Refused`] when the chat is not loaded.
    pub fn resume_chat(
        &self,
        chat: &str,
    ) -> Result<(), Error> {
        self.run(self.hub.read(|state| plan_agent::resume(state, chat)))
    }

    fn plan_and_run(
        &self,
        command: &Command,
    ) -> Result<(), Error> {
        self.run(
            self.hub
                .read(|state| planner::plan((state, self.files.as_ref()), command)),
        )
    }

    fn run(
        &self,
        plan: Result<Plan, PlanError>,
    ) -> Result<(), Error> {
        let plan = plan.map_err(Error::from)?;
        let (hub, outcome) = (Arc::clone(&self.hub), Arc::clone(&self.outcome));
        self.handle
            .spawn(async move { runner::run(&hub, plan, &outcome).await });
        Ok(())
    }
}

impl Sink for SidebarCommands {
    fn dispatch(
        &self,
        command: Command,
    ) -> Result<(), Error> {
        match &command {
            Command::SelectHost(host) => self.select_host(host.as_str()),
            Command::SelectProject(project) => {
                self.hub
                    .edit(|state| state.active_project = Some(project.as_str().to_owned()));
            },
            Command::RetryChats(project) => self.retry(project.as_str()),
            Command::ToggleDirectory(path) => self.toggle(path),
            other => {
                return self
                    .file_command(other)
                    .unwrap_or_else(|| self.plan_and_run(other));
            },
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::followed_commands;
    use sidebar::{HostId, ProjectId};

    fn commands() -> (SidebarCommands, Arc<crate::test_support::FakeDaemon>) {
        followed_commands(Arc::new(|_| {}), &Arc::default())
    }

    async fn until_listed(commands: &SidebarCommands) {
        while commands.hub.read(|state| state.panels.listings.is_empty()) {
            tokio::task::yield_now().await;
        }
    }

    #[tokio::test]
    async fn selecting_a_host_or_a_space_changes_what_is_shown() {
        let (commands, _) = commands();
        commands
            .dispatch(Command::SelectProject(ProjectId::from("p")))
            .unwrap();
        assert_eq!(
            commands
                .hub
                .read(|state| state.active_project.clone())
                .as_deref(),
            Some("p")
        );
        commands
            .dispatch(Command::SelectHost(HostId::from("local")))
            .unwrap();
        assert!(commands.hub.read(|state| state.active_project.is_none()));
    }

    #[tokio::test]
    async fn unfolding_a_directory_lists_it_once() {
        let (commands, daemon) = commands();
        daemon.ok(
            "GET",
            "/v0/chats/hc/files/tree?path=src",
            r#"[{"name":"m.rs","path":"src/m.rs","type":"file"}]"#,
        );
        commands
            .dispatch(Command::ToggleDirectory("src".into()))
            .unwrap();
        until_listed(&commands).await;
        commands
            .dispatch(Command::ToggleDirectory("src".into()))
            .unwrap();
        commands
            .dispatch(Command::ToggleDirectory("src".into()))
            .unwrap();
        let listed = daemon
            .seen()
            .iter()
            .filter(|call| call.path.contains("path=src"))
            .count();
        assert_eq!(listed, 1);
    }

    fn assert_all_refused(
        commands: &SidebarCommands,
        cases: [Command; 6],
    ) {
        for command in cases {
            assert!(commands.dispatch(command).is_err());
        }
    }

    #[tokio::test]
    async fn the_shells_own_commands_are_refused() {
        let (commands, _) = commands();
        let cases = [
            Command::OpenSettings,
            Command::OpenHostSettings(HostId::from("local")),
            Command::OpenRow(sidebar::RowId::from("r")),
            Command::ImportRepo,
            Command::ImportBranches(sidebar::RowId::from("r")),
            Command::OpenFile("a".into()),
        ];
        assert_all_refused(&commands, cases);
    }
}
