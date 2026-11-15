//! Making an env template next to an env file of the followed workspace.

use std::sync::Arc;

use sidebar::Error;

use crate::env_template::template_of;
use crate::file_done::FileDone;
use crate::file_session::{FileSession, join, parent_of};
use crate::hub::Hub;
use crate::overwrite::Overwrite;
use crate::sidebar_commands::SidebarCommands;
use crate::template_outcome::TemplateOutcome;

async fn make(
    (hub, session): (&Hub, &FileSession),
    (source, target): (&str, &str),
    overwrite: Overwrite,
) -> Result<TemplateOutcome, String> {
    let directory = parent_of(source);
    let path = join(&directory, target);
    if path == source {
        return Err(format!("{target} is the file the template is made from"));
    }
    let exists = session
        .exists(&path)
        .await
        .map_err(|error| error.to_string())?;
    if exists && overwrite == Overwrite::Ask {
        return Ok(TemplateOutcome::Exists(path));
    }
    let text = session
        .read_text(source)
        .await
        .map_err(|error| error.to_string())?;
    if !exists {
        session
            .create_file(&path)
            .await
            .map_err(|error| error.to_string())?;
    }
    session
        .write_text(&path, &template_of(&text))
        .await
        .map_err(|error| error.to_string())?;
    session.reload(hub, &directory).await;
    Ok(TemplateOutcome::Created(path))
}

impl SidebarCommands {
    /// Writes `target` next to the env file `source` (workspace-relative), with `source`'s
    /// values emptied. When `target` exists and `overwrite` is [`Overwrite::Ask`], nothing is
    /// written and `done` gets [`TemplateOutcome::Exists`].
    ///
    /// # Errors
    /// [`Error::Refused`] when no workspace is open.
    pub fn create_env_template(
        &self,
        (source, target): (&str, &str),
        overwrite: Overwrite,
        done: FileDone<TemplateOutcome>,
    ) -> Result<(), Error> {
        let session = FileSession::of(&self.hub).map_err(|reason| Error::Refused { reason })?;
        let hub = Arc::clone(&self.hub);
        let (source, target) = (source.to_owned(), target.to_owned());
        self.handle.spawn(async move {
            done(make((&hub, &session), (&source, &target), overwrite).await);
        });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use serde_json::json;
    use tokio::runtime::Handle;

    use super::*;
    use crate::fleet::Fleet;
    use crate::test_support::{FakeDaemon, state};

    const SOURCE_ROUTE: &str = "/v0/documents/content?workspace=hw&path=app/.env";
    const TARGET_ROUTE: &str = "/v0/documents/content?workspace=hw&path=app/.env.example";

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>) {
        let (mut state, daemon) = state();
        state.panels.follow(Some("hc".into()));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let hub = Hub::new(state, Arc::new(|| {}));
        let commands = SidebarCommands {
            fleet: Fleet::new(&Handle::current(), &hub, &transport),
            hub,
            handle: Handle::current(),
            outcome: Arc::new(|_| {}),
        };
        daemon.ok("GET", SOURCE_ROUTE, r#"{"content":"KEY=secret # keep\n"}"#);
        daemon.ok("POST", "/v0/chats/hc/files", "{}");
        daemon.ok("PUT", "/v0/documents/content", "{}");
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=app", "[]");
        (commands, daemon)
    }

    async fn run(
        commands: &SidebarCommands,
        (source, target): (&str, &str),
        overwrite: Overwrite,
    ) -> Result<TemplateOutcome, String> {
        let (sender, receiver) = mpsc::channel();
        let done: FileDone<TemplateOutcome> = Box::new(move |result| {
            sender.send(result).unwrap();
        });
        commands
            .create_env_template((source, target), overwrite, done)
            .unwrap();
        tokio::task::spawn_blocking(move || receiver.recv().unwrap())
            .await
            .unwrap()
    }

    fn writes(daemon: &FakeDaemon) -> Vec<serde_json::Value> {
        let seen = daemon.seen();
        let puts = seen.iter().filter(|call| call.method == "PUT");
        puts.filter_map(|call| call.body.clone()).collect()
    }

    #[tokio::test]
    async fn a_template_is_created_and_an_existing_one_is_asked_about_before_it_is_replaced() {
        let (commands, daemon) = commands();
        let pair = ("app/.env", ".env.example");
        let made = run(&commands, pair, Overwrite::Ask).await;
        assert_eq!(
            made,
            Ok(TemplateOutcome::Created("app/.env.example".into()))
        );
        let first =
            json!({"workspaceId": "hw", "path": "app/.env.example", "content": "KEY= # keep\n"});
        assert_eq!(writes(&daemon), std::slice::from_ref(&first));
        daemon.ok("GET", TARGET_ROUTE, r#"{"content":"OLD=1"}"#);
        let asked = run(&commands, pair, Overwrite::Ask).await;
        assert_eq!(
            asked,
            Ok(TemplateOutcome::Exists("app/.env.example".into()))
        );
        assert_eq!(
            writes(&daemon),
            std::slice::from_ref(&first),
            "nothing is written before the answer"
        );
        let replaced = run(&commands, pair, Overwrite::Replace).await;
        assert_eq!(
            replaced,
            Ok(TemplateOutcome::Created("app/.env.example".into()))
        );
        assert_eq!(writes(&daemon), [first.clone(), first]);
        let created = daemon
            .seen()
            .iter()
            .filter(|call| call.method == "POST")
            .count();
        assert_eq!(created, 1, "an existing file is not created again");
    }

    #[tokio::test]
    async fn a_template_of_itself_or_of_an_unreadable_source_is_refused_unwritten() {
        let (commands, daemon) = commands();
        let own = run(&commands, ("app/.env", ".env"), Overwrite::Replace).await;
        let unread = run(
            &commands,
            ("app/.env.missing", ".env.example"),
            Overwrite::Ask,
        )
        .await;
        assert!(own.is_err() && unread.is_err());
        assert_eq!(writes(&daemon), Vec::<serde_json::Value>::new());
    }
}
