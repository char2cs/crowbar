//! The Files panel's changes to the workspace's files, each a daemon file route.

use std::sync::Arc;

use daemonapi::Call;
use serde_json::json;
use sidebar::Error;

use crate::live::DocumentBody;
use crate::live::Hub;
use crate::live::PlanError;
use crate::live::ReloadTarget;
use crate::live::{FileTarget, panel_loader::file_target};
use crate::live::{Outcome, OutcomeSink};
use crate::plan::Plan;
use crate::plan::runner;
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_commands::file_session::{PATH_KEY, parent_of};
use crate::sidebar_commands::new_entry::NewEntry;

fn files_route(target: &FileTarget) -> String {
    format!("/v0/chats/{}/files", target.chat)
}

async fn change_and_reload(
    hub: &Hub,
    plan: Plan,
    outcome: &OutcomeSink,
    reloads: Vec<ReloadTarget>,
) {
    runner::run(hub, plan, outcome).await;
    for reload in reloads {
        reload.load(hub).await;
    }
}

async fn say_content(
    client: &daemonapi::Client,
    route: daemonapi::Route,
    outcome: &OutcomeSink,
) {
    let said = match client.read::<DocumentBody>(route).await {
        Ok(body) => Outcome::CopyText(body.content),
        Err(error) => Outcome::Refused {
            title: "Copy the file content".to_owned(),
            reason: error.to_string(),
        },
    };
    outcome(said);
}

impl SidebarCommands {
    fn change_files(
        &self,
        failure: Option<&str>,
        make: impl FnOnce(&FileTarget) -> (Call, Vec<String>),
    ) -> Result<(), Error> {
        let target = self.hub.read(file_target::of).map_err(Error::from)?;
        let (call, mut directories) = make(&target);
        directories.dedup();
        let plan = Plan::of(&target.host, vec![call]).failing_as(failure.map(str::to_owned));
        let (hub, outcome) = (Arc::clone(&self.hub), Arc::clone(&self.outcome));
        let client = self.hub.read(|state| state.client_of_chat(&target.chat));
        let reload = |directory| ReloadTarget {
            client: client.clone(),
            chat: target.chat.clone(),
            directory,
        };
        let reloads = directories.into_iter().map(reload).collect();
        self.handle
            .spawn(async move { change_and_reload(&hub, plan, &outcome, reloads).await });
        Ok(())
    }

    pub(crate) fn create_entry(
        &self,
        path: &str,
        entry: NewEntry,
    ) -> Result<(), Error> {
        self.change_files(None, |target| {
            let body = json!({PATH_KEY: path, "type": entry.kind()});
            (
                Call::post(files_route(target)).json(body),
                vec![parent_of(path)],
            )
        })
    }

    pub(crate) fn duplicate_file(
        &self,
        (source, destination): (&str, &str),
        failure: &str,
    ) -> Result<(), Error> {
        self.change_files(Some(failure), |target| {
            let body = json!({"sourcePath": source, "destPath": destination});
            let route = format!("{}/copy", files_route(target));
            (Call::post(route).json(body), vec![parent_of(destination)])
        })
    }

    pub(crate) fn rename_file(
        &self,
        path: &str,
        new_path: &str,
    ) -> Result<(), Error> {
        self.change_files(Some("Rename failed"), |target| {
            let body = json!({PATH_KEY: path, "newPath": new_path});
            let left_and_entered = vec![parent_of(path), parent_of(new_path)];
            (
                Call::patch(files_route(target)).json(body),
                left_and_entered,
            )
        })
    }

    pub(crate) fn delete_file(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        self.change_files(None, |target| {
            let body = json!({PATH_KEY: path});
            (
                Call::delete(files_route(target)).json(body),
                vec![parent_of(path)],
            )
        })
    }

    pub(crate) fn copy_file_content(
        &self,
        path: &str,
    ) -> Result<(), Error> {
        let target = self.hub.read(file_target::of).map_err(Error::from)?;
        let client = self
            .hub
            .read(|state| state.client_of_chat(&target.chat))
            .ok_or(PlanError::WorkspaceNotLoaded)?;
        let route = daemonapi::Route::document_content(&target.workspace, path);
        let outcome = Arc::clone(&self.outcome);
        self.handle
            .spawn(async move { say_content(&client, route, &outcome).await });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use super::*;
    use crate::test_support::{FakeDaemon, followed_commands};

    type Said = Arc<Mutex<Vec<Outcome>>>;

    fn commands() -> (SidebarCommands, Arc<FakeDaemon>, Said) {
        let said = Said::default();
        let sink = Arc::clone(&said);
        let outcome: OutcomeSink = Arc::new(move |outcome| sink.lock().unwrap().push(outcome));
        let (commands, daemon) = followed_commands(outcome, &Arc::default());
        (commands, daemon, said)
    }

    async fn until_seen(
        daemon: &FakeDaemon,
        method: &str,
        path: &str,
    ) -> Option<serde_json::Value> {
        let mut body = None;
        while body.is_none() {
            let call = daemon
                .seen()
                .into_iter()
                .find(|call| call.method == method && call.path == path);
            body = call.and_then(|call| call.body);
            tokio::task::yield_now().await;
        }
        body
    }

    async fn until_said(
        said: &Said,
        count: usize,
    ) -> Vec<Outcome> {
        while said.lock().unwrap().len() < count {
            tokio::task::yield_now().await;
        }
        said.lock().unwrap().clone()
    }

    #[tokio::test]
    async fn each_file_change_calls_its_route_with_workspace_relative_paths() {
        let (commands, daemon, _) = commands();
        let files = "/v0/chats/hc/files";
        daemon.ok("POST", files, "{}");
        daemon.ok("POST", &format!("{files}/copy"), "{}");
        daemon.ok("PATCH", files, "{}");
        daemon.ok("DELETE", files, "{}");
        daemon.ok("GET", &format!("{files}/tree?path=src"), "[]");
        commands
            .create_entry("src/new", NewEntry::Directory)
            .unwrap();
        commands
            .duplicate_file(("src/a", "src/b"), "Duplicate failed")
            .unwrap();
        commands.rename_file("src/b", "src/c").unwrap();
        commands.delete_file("src/c").unwrap();
        let made = until_seen(&daemon, "POST", files).await;
        let copied = until_seen(&daemon, "POST", &format!("{files}/copy")).await;
        let moved = until_seen(&daemon, "PATCH", files).await;
        let gone = until_seen(&daemon, "DELETE", files).await;
        assert_eq!(made, Some(json!({"path": "src/new", "type": "dir"})));
        assert_eq!(
            copied,
            Some(json!({"sourcePath": "src/a", "destPath": "src/b"}))
        );
        assert_eq!(moved, Some(json!({"path": "src/b", "newPath": "src/c"})));
        assert_eq!(gone, Some(json!({"path": "src/c"})));
    }

    async fn listed_after(
        daemon: &FakeDaemon,
        last: &str,
    ) -> Vec<String> {
        let tree = "/v0/chats/hc/files/tree?path=";
        let listed = || -> Vec<String> {
            let calls = daemon
                .seen()
                .into_iter()
                .filter(|call| call.method == "GET");
            let paths = calls.filter_map(|call| call.path.strip_prefix(tree).map(str::to_owned));
            paths.collect()
        };
        while !listed().iter().any(|path| path == last) {
            tokio::task::yield_now().await;
        }
        listed()
    }

    fn serve_listings(
        daemon: &FakeDaemon,
        directories: &[&str],
    ) {
        for directory in directories {
            daemon.ok(
                "GET",
                &format!("/v0/chats/hc/files/tree?path={directory}"),
                "[]",
            );
        }
    }

    type Move = (&'static str, &'static str, &'static [&'static str]);

    const MOVE_CASES: [Move; 2] = [
        ("src/a.rs", "lib/a.rs", &["src", "lib"]),
        ("src/a.rs", "src/b.rs", &["src"]),
    ];

    async fn listed_after_move((path, new_path, want): Move) -> Vec<String> {
        let (commands, daemon, _) = commands();
        daemon.ok("PATCH", "/v0/chats/hc/files", "{}");
        serve_listings(&daemon, want);
        commands.rename_file(path, new_path).unwrap();
        listed_after(&daemon, want.last().copied().unwrap_or_default()).await
    }

    #[test]
    fn a_move_lists_again_the_folder_it_left_and_the_one_it_entered() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        for case in MOVE_CASES {
            let listed = runtime.block_on(listed_after_move(case));
            assert_eq!(listed, case.2, "{}", case.1);
        }
    }

    #[tokio::test]
    async fn a_files_content_reaches_the_shell_and_a_refusal_is_said_under_its_title() {
        let (commands, daemon, said) = commands();
        daemon.ok(
            "GET",
            "/v0/documents/content?workspace=hw&path=a%20b.txt",
            r#"{"content":"hello","encoding":"utf-8"}"#,
        );
        commands.copy_file_content("a b.txt").unwrap();
        assert_eq!(
            until_said(&said, 1).await[0],
            Outcome::CopyText("hello".into())
        );
        commands.copy_file_content("missing").unwrap();
        let heard = until_said(&said, 2).await;
        assert!(
            matches!(&heard[1], Outcome::Refused { title, .. } if title == "Copy the file content")
        );
        daemon.answer(
            ("PATCH", "/v0/chats/hc/files"),
            409,
            r#"{"success":false,"error":"src/c is taken"}"#,
        );
        daemon.ok("GET", "/v0/chats/hc/files/tree?path=src", "[]");
        commands.rename_file("src/b", "src/c").unwrap();
        let refused = Outcome::Refused {
            title: "Rename failed".into(),
            reason: "src/c is taken".into(),
        };
        assert_eq!(until_said(&said, 3).await[2], refused);
    }
}
