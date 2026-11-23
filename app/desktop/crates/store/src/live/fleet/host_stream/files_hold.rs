//! The `files:` scope one connection holds for the followed chat's workspace, as develop's
//! `use-workspace-file-tree.ts` does, and what that scope's frames change in the panels.

use std::sync::Arc;

use daemonapi::Frame;
use serde_json::Value;
use tokio::sync::{Notify, mpsc};
use tokio::task::JoinHandle;

use crate::live::fleet::host_stream::file_change::FileChange;
use crate::live::fleet::host_stream::files_wire::{
    listed_of, load_all, reload_git_when_told, send, wanted_scope,
};
use crate::live::state::Hub;
use crate::live::{Outcome, OutcomeSink};

const FILE_CHANGE: &str = "file_change";
const ROOT: &str = "";
const RESYNC: &str = "resync";

pub(crate) struct FilesHold {
    hub: Arc<Hub>,
    host: String,
    held: Option<String>,
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
            held: None,
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
                self.signal(kind, data.clone());
            },
            Frame::Synced { scope, .. } if self.holds(scope) => self.synced(),
            Frame::Closed { scope, reason, .. } if self.holds(scope) && reason == RESYNC => {
                self.resubscribe(outgoing);
            },
            _ => return false,
        }
        true
    }

    fn holds(
        &self,
        scope: &str,
    ) -> bool {
        self.held.as_deref() == Some(scope)
    }

    pub(crate) fn sync(
        &mut self,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        let wanted = self.hub.read(|state| wanted_scope(state, &self.host));
        if wanted == self.held {
            return;
        }
        if let Some(released) = self.held.take() {
            send(outgoing, ("unsub", &released));
        }
        if let Some(held) = &wanted {
            send(outgoing, ("sub", held));
        }
        self.held = wanted;
    }

    fn resubscribe(
        &self,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        if let Some(held) = &self.held {
            send(outgoing, ("sub", held));
        }
    }

    fn synced(&self) {
        self.relist(vec![ROOT.to_owned()]);
        if let Some(workspace) = self.workspace() {
            (self.outcome)(Outcome::FilesSynced { workspace });
        }
    }

    fn workspace(&self) -> Option<String> {
        let held = self.held.as_deref()?;
        held.split_once(':')
            .map(|(_, workspace)| workspace.to_owned())
    }

    fn tell_changed(
        &self,
        change: &FileChange,
    ) {
        if !change.changes_text() {
            return;
        }
        let Some(workspace) = self.workspace() else {
            return;
        };
        let path = change.path.clone();
        (self.outcome)(Outcome::FileChanged { workspace, path });
    }

    fn signal(
        &self,
        kind: &str,
        data: Option<Value>,
    ) {
        if kind != FILE_CHANGE {
            return;
        }
        let change = data
            .and_then(|data| serde_json::from_value::<FileChange>(data).ok())
            .unwrap_or_default();
        self.tell_changed(&change);
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
        FilesHold::sync_held(&mut files, &outgoing);
        let frames = [
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"modified","path":"src/a.ts"}}"#,
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"created","path":"b.ts"}}"#,
            r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"deleted","path":"c.ts"}}"#,
            r#"{"t":"synced","scope":"files:hw"}"#,
        ];
        told(files.as_ref().unwrap(), &frames, &outgoing);
        let changed = |path: &str| Outcome::FileChanged {
            workspace: "hw".into(),
            path: path.into(),
        };
        let synced = Outcome::FilesSynced {
            workspace: "hw".into(),
        };
        assert_eq!(
            *heard.lock().unwrap(),
            [changed("src/a.ts"), changed("b.ts"), synced]
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
        FilesHold::sync_held(&mut files, &outgoing);
        assert_eq!(held, Some((true, false)));
        assert_eq!(
            drain(sent),
            [
                r#"{"op":"sub","scope":"files:hw"}"#,
                r#"{"op":"sub","scope":"files:hw"}"#,
                r#"{"op":"unsub","scope":"files:hw"}"#,
            ],
            "held once, asked again on a resync, released on unfollow"
        );
    }
}
