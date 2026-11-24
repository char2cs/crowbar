//! The `files:` scopes one connection holds for the workspace of every open chat, as develop's
//! `use-workspace-file-tree.ts` does, and what their frames change in the panels and the shell.

use std::collections::BTreeSet;
use std::sync::Arc;

use daemonapi::Frame;
use serde_json::Value;
use tokio::sync::{Notify, mpsc};
use tokio::task::JoinHandle;

use crate::live::fleet::host_stream::file_change::FileChange;
use crate::live::fleet::host_stream::files_wire::{
    followed_scope, listed_of, load_all, reload_git_when_told, send, wanted_scopes,
};
use crate::live::state::Hub;
use crate::live::{Outcome, OutcomeSink};

const FILE_CHANGE: &str = "file_change";
const ROOT: &str = "";
const RESYNC: &str = "resync";

pub(crate) struct FilesHold {
    hub: Arc<Hub>,
    host: String,
    held: BTreeSet<String>,
    git: Arc<Notify>,
    reloader: JoinHandle<()>,
    outcome: OutcomeSink,
}

impl FilesHold {
    pub(crate) fn new(
        hub: &Arc<Hub>,
        host: &str,
        outcome: OutcomeSink,
    ) -> Self {
        let git = Arc::new(Notify::new());
        let reloader = tokio::spawn(reload_git_when_told(Arc::clone(hub), Arc::clone(&git)));
        Self {
            hub: Arc::clone(hub),
            host: host.to_owned(),
            held: BTreeSet::new(),
            git,
            reloader,
            outcome,
        }
    }

    pub(crate) fn sync_held(
        files: &mut Option<Self>,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        if let Some(held) = files {
            held.sync(outgoing);
        }
    }

    pub(crate) fn hear(
        &self,
        frame: &Frame,
        outgoing: &mpsc::UnboundedSender<String>,
    ) -> bool {
        match frame {
            Frame::Signal { scope, kind, data } if self.holds(scope) => {
                self.signal(scope, (kind, data.clone()));
            },
            Frame::Synced { scope, .. } if self.holds(scope) => self.synced(scope),
            Frame::Closed { scope, reason, .. } if self.holds(scope) && reason == RESYNC => {
                send(outgoing, ("sub", scope));
            },
            _ => return false,
        }
        true
    }

    fn holds(
        &self,
        scope: &str,
    ) -> bool {
        self.held.contains(scope)
    }

    fn followed(
        &self,
        scope: &str,
    ) -> bool {
        let followed = self.hub.read(|state| followed_scope(state, &self.host));
        followed.as_deref() == Some(scope)
    }

    pub(crate) fn sync(
        &mut self,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        let wanted = self.hub.read(|state| wanted_scopes(state, &self.host));
        for released in self.held.difference(&wanted) {
            send(outgoing, ("unsub", released));
        }
        for held in wanted.difference(&self.held) {
            send(outgoing, ("sub", held));
        }
        self.held = wanted;
    }

    fn synced(
        &self,
        scope: &str,
    ) {
        if self.followed(scope) {
            self.relist(vec![ROOT.to_owned()]);
        }
        let workspace = workspace_of(scope);
        (self.outcome)(Outcome::FilesSynced { workspace });
    }

    fn tell_changed(
        &self,
        (scope, change): (&str, &FileChange),
    ) {
        if !change.changes_text() {
            return;
        }
        let (workspace, path) = (workspace_of(scope), change.path.clone());
        (self.outcome)(Outcome::FileChanged { workspace, path });
    }

    fn signal(
        &self,
        scope: &str,
        (kind, data): (&str, Option<Value>),
    ) {
        if kind != FILE_CHANGE {
            return;
        }
        let change = data
            .and_then(|data| serde_json::from_value::<FileChange>(data).ok())
            .unwrap_or_default();
        self.tell_changed((scope, &change));
        if !self.followed(scope) {
            return;
        }
        self.relist(change.folders());
        self.git.notify_one();
    }

    fn relist(
        &self,
        folders: Vec<String>,
    ) {
        let targets = self.hub.read(|state| listed_of(state, folders));
        let hub = Arc::clone(&self.hub);
        tokio::spawn(async move { load_all(&hub, targets).await });
    }
}

fn workspace_of(scope: &str) -> String {
    let workspace = scope.split_once(':').map(|(_, workspace)| workspace);
    workspace.unwrap_or(scope).to_owned()
}

impl Drop for FilesHold {
    fn drop(&mut self) {
        self.reloader.abort();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::live::Outcome;
    use crate::test_support::state;

    const HELD: &str = r#"{"t":"closed","scope":"files:hw","reason":"resync"}"#;
    const OTHER: &str = r#"{"t":"synced","scope":"files:elsewhere"}"#;

    fn unheard(_: Outcome) {}

    fn quiet_hub() -> Arc<Hub> {
        Hub::new(state().0, Arc::new(|| {}))
    }

    fn heard(
        files: &FilesHold,
        outgoing: &mpsc::UnboundedSender<String>,
    ) -> (bool, bool) {
        let held = Frame::decode(HELD).unwrap();
        let other = Frame::decode(OTHER).unwrap();
        (files.hear(&held, outgoing), files.hear(&other, outgoing))
    }

    fn drain(mut sent: mpsc::UnboundedReceiver<String>) -> Vec<String> {
        std::iter::from_fn(|| sent.try_recv().ok()).collect()
    }

    fn told(
        files: &FilesHold,
        frames: &[&str],
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        for frame in frames {
            files.hear(&Frame::decode(frame).unwrap(), outgoing);
        }
    }

    #[tokio::test]
    async fn an_edit_or_new_file_on_disk_and_each_resync_reach_the_shell() {
        let hub = quiet_hub();
        let heard = Arc::new(std::sync::Mutex::new(Vec::new()));
        let log = Arc::clone(&heard);
        let outcome: OutcomeSink = Arc::new(move |outcome| log.lock().unwrap().push(outcome));
        let (outgoing, _sent) = mpsc::unbounded_channel();
        let mut files = Some(FilesHold::new(&hub, "local", outcome));
        hub.edit(|state| state.panels.follow(Some("hc".into())));
        hub.edit(|state| state.watched_chats = ["bo".to_owned()].into());
        FilesHold::sync_held(&mut files, &outgoing);
        let frames = [
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"modified","path":"src/a.ts"}}"#,
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"created","path":"b.ts"}}"#,
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"deleted","path":"c.ts"}}"#,
            r#"{"t":"synced","scope":"files:hw"}"#,
            r#"{"t":"signal","scope":"files:b","kind":"file_change","data":{"type":"modified","path":"src/x.ts"}}"#,
        ];
        told(files.as_ref().unwrap(), &frames, &outgoing);
        let changed = |(workspace, path): (&str, &str)| Outcome::FileChanged {
            workspace: workspace.into(),
            path: path.into(),
        };
        let synced = Outcome::FilesSynced {
            workspace: "hw".into(),
        };
        let unfocused = changed(("b", "src/x.ts"));
        assert_eq!(
            *heard.lock().unwrap(),
            [
                changed(("hw", "src/a.ts")),
                changed(("hw", "b.ts")),
                synced,
                unfocused
            ],
            "the focused chat's workspace and every open chat's"
        );
    }

    #[tokio::test]
    async fn the_hold_follows_the_followed_chat_and_hears_only_its_own_scope() {
        let hub = quiet_hub();
        let (outgoing, sent) = mpsc::unbounded_channel();
        let mut files = Some(FilesHold::new(&hub, "local", Arc::new(unheard)));
        hub.edit(|state| state.panels.follow(Some("hc".into())));
        FilesHold::sync_held(&mut files, &outgoing);
        FilesHold::sync_held(&mut files, &outgoing);
        let held = files.as_ref().map(|files| heard(files, &outgoing));
        hub.edit(|state| state.panels.follow(None));
        hub.edit(|state| state.watched_chats = ["bo".to_owned()].into());
        FilesHold::sync_held(&mut files, &outgoing);
        hub.edit(|state| state.watched_chats.clear());
        FilesHold::sync_held(&mut files, &outgoing);
        assert_eq!(held, Some((true, false)));
        assert_eq!(
            drain(sent),
            [
                r#"{"op":"sub","scope":"files:hw"}"#,
                r#"{"op":"sub","scope":"files:hw"}"#,
                r#"{"op":"unsub","scope":"files:hw"}"#,
                r#"{"op":"sub","scope":"files:b"}"#,
                r#"{"op":"unsub","scope":"files:b"}"#,
            ],
            "held once, asked again on a resync, released on unfollow; an open chat's likewise"
        );
    }
}
