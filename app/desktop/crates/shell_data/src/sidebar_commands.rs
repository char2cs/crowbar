//! The sidebar's write side: every command that has a daemon route.

use std::sync::Arc;

use sidebar::{Command, CommandSink, Error};
use tokio::runtime::Handle;
use tokio::sync::mpsc;

use crate::hub::Hub;
use crate::new_id;
use crate::outcome::OutcomeSink;
use crate::panel_loader;
use crate::plan::Plan;
use crate::plan_create;
use crate::planner;
use crate::runner;
use crate::sidebar_reads::{active_project, host_of};
use crate::stream_control::{Controls, StreamControl};

/// Plans each command from the current entities and runs it on the runtime; the stream shows the
/// result. Commands that are the shell's own (dialogs, opening a pane) are refused here.
#[derive(Clone)]
pub struct SidebarCommands {
    pub(crate) hub: Arc<Hub>,
    pub(crate) handle: Handle,
    pub(crate) outcome: OutcomeSink,
    pub(crate) controls: Arc<Controls>,
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

fn refused(reason: impl Into<String>) -> Error {
    Error::Refused {
        reason: reason.into(),
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
        self.controls
            .iter()
            .filter(|(id, _)| host.as_ref().is_none_or(|held| held == id))
            .for_each(|(id, control)| resubscribe(id, control));
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

    /// Adds the repository at `path`, a folder of this computer, to the open space.
    ///
    /// # Errors
    /// [`Error::Refused`] when no space is open or its host is another computer.
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

    fn plan_and_run(
        &self,
        command: &Command,
    ) -> Result<(), Error> {
        self.run(self.hub.read(|state| planner::plan(state, command)))
    }

    fn run(
        &self,
        plan: Result<Plan, String>,
    ) -> Result<(), Error> {
        let plan = plan.map_err(refused)?;
        let (hub, outcome) = (Arc::clone(&self.hub), Arc::clone(&self.outcome));
        self.handle
            .spawn(async move { runner::run(&hub, plan, &outcome).await });
        Ok(())
    }
}

fn resubscribe(
    id: &str,
    control: &mpsc::UnboundedSender<StreamControl>,
) {
    if control.send(StreamControl::Resubscribe).is_err() {
        tracing::warn!(host = %id, "the stream task is gone; the chats cannot be reloaded");
    }
}

impl CommandSink for SidebarCommands {
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
            other => return self.plan_and_run(other),
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell_data::ShellData;
    use crate::test_support::state;
    use sidebar::{HostId, ProjectId};

    fn commands() -> (SidebarCommands, Arc<crate::test_support::FakeDaemon>) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let hub = Hub::new(state, Arc::new(|| {}));
        let commands = SidebarCommands {
            hub,
            handle: Handle::current(),
            outcome: Arc::new(|_| {}),
            controls: Arc::new(Vec::new()),
        };
        (commands, daemon)
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
        assert!(!cases.iter().any(ShellData::serves));
        assert!(
            !cases
                .into_iter()
                .any(|command| commands.dispatch(command).is_ok())
        );
    }
}
