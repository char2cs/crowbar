//! Following one host's `/v0/stream`: connect, subscribe, apply frames, reconnect.

mod backoff;
mod ended;
mod icon_ask;
mod icon_fetch;
mod step;
mod terminal_offer;

use std::sync::Arc;

use daemonapi::{Connection, Dial, Frame};
use serde_json::json;
use tokio::sync::mpsc;

use crate::live::fleet::host_stream::backoff::Backoff;
use crate::live::fleet::host_stream::ended::Ended;
use crate::live::fleet::host_stream::step::{Step, next_step};
use crate::live::fleet::host_stream::terminal_offer::read_terminal_support;
use crate::live::fleet::stream_control::StreamControl;
use crate::live::held::LinkState;
use crate::live::held::PROJECTS_SCOPE;
use crate::live::panel_loader::changes_reload;
use crate::live::state::Hub;

const RESYNC: &str = "resync";

pub(crate) struct HostStream {
    pub(crate) hub: Arc<Hub>,
    pub(crate) host: String,
    pub(crate) dial: Arc<dyn Dial>,
    pub(crate) client: daemonapi::Client,
}

impl HostStream {
    pub(crate) async fn run(
        self,
        mut controls: mpsc::UnboundedReceiver<StreamControl>,
    ) {
        let mut backoff = Backoff::default();
        while !matches!(
            self.cycle(&mut controls, &mut backoff).await,
            Ended::Stopped
        ) {}
    }

    async fn cycle(
        &self,
        controls: &mut mpsc::UnboundedReceiver<StreamControl>,
        backoff: &mut Backoff,
    ) -> Ended {
        self.set_link(LinkState::Connecting);
        let ended = self.connect_once(controls, backoff).await;
        if matches!(ended, Ended::Stopped) {
            return ended;
        }
        self.set_link(LinkState::Down);
        tokio::time::sleep(backoff.wait()).await;
        ended
    }

    async fn connect_once(
        &self,
        controls: &mut mpsc::UnboundedReceiver<StreamControl>,
        backoff: &mut Backoff,
    ) -> Ended {
        match self.dial.open().await {
            Ok(connection) => self.session(connection, controls, backoff).await,
            Err(error) => {
                tracing::warn!(host = %self.host, %error, "the stream could not be opened");
                Ended::Dropped
            },
        }
    }

    fn set_link(
        &self,
        link: LinkState,
    ) {
        self.hub.edit(|state| state.link(&self.host, link));
    }

    async fn session(
        &self,
        connection: Connection,
        controls: &mut mpsc::UnboundedReceiver<StreamControl>,
        backoff: &mut Backoff,
    ) -> Ended {
        let Connection {
            mut incoming,
            outgoing,
        } = connection;
        loop {
            match next_step(&mut incoming, controls).await {
                Step::Frame(None) => return Ended::Dropped,
                Step::Frame(Some(text)) => self.on_text(&text, &outgoing, backoff),
                Step::Control(Some(StreamControl::Resubscribe)) => self.subscribe(&outgoing),
                Step::Control(None) => return Ended::Stopped,
            }
        }
    }

    fn on_text(
        &self,
        text: &str,
        outgoing: &mpsc::UnboundedSender<String>,
        backoff: &mut Backoff,
    ) {
        let Some(frame) = self.decode(text) else {
            return;
        };
        if matches!(frame, Frame::Ready { .. }) {
            backoff.reset();
        }
        self.react(frame, outgoing);
    }

    fn decode(
        &self,
        text: &str,
    ) -> Option<Frame> {
        Frame::decode(text)
            .inspect_err(|error| {
                tracing::warn!(host = %self.host, %error, "a stream frame was unreadable and skipped");
            })
            .ok()
    }

    fn react(
        &self,
        frame: Frame,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        match &frame {
            Frame::Ready { .. } => self.on_ready(frame, outgoing),
            Frame::Closed { scope, reason, .. } if scope == PROJECTS_SCOPE && reason == RESYNC => {
                self.subscribe(outgoing);
            },
            Frame::Closed {
                scope,
                reason,
                problem,
            } => {
                tracing::warn!(host = %self.host, scope, reason, ?problem, "the daemon ended a stream scope");
            },
            Frame::Error { problem } => {
                tracing::warn!(host = %self.host, ?problem, "the daemon refused a stream request");
            },
            _ => self.apply(frame),
        }
    }

    fn on_ready(
        &self,
        frame: Frame,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        self.set_link(LinkState::Live);
        self.apply(frame);
        self.subscribe(outgoing);
        self.learn_providers();
    }

    fn learn_providers(&self) {
        let hub = Arc::clone(&self.hub);
        let host = self.host.clone();
        let client = self.client.clone();
        tokio::spawn(async move {
            read_terminal_support(&hub, &host, &client).await;
        });
    }

    fn apply(
        &self,
        frame: Frame,
    ) {
        let operation = changes_reload::is_operation(&frame);
        self.hub.edit(|state| state.apply(&self.host, frame));
        if operation {
            changes_reload::follow(&self.hub);
        }
        icon_fetch::follow(&self.hub, &self.host, &self.client);
    }

    fn subscribe(
        &self,
        outgoing: &mpsc::UnboundedSender<String>,
    ) {
        self.hub.edit(|state| state.begin_snapshot(&self.host));
        let request = json!({"op": "sub", "scope": PROJECTS_SCOPE}).to_string();
        if outgoing.send(request).is_err() {
            tracing::debug!(host = %self.host, "the stream closed before the subscription went out");
        }
    }
}

#[cfg(test)]
mod tests {
    use std::collections::VecDeque;
    use std::sync::{Arc, Mutex};

    use tokio::sync::Notify;

    use super::*;
    use daemonapi::Opening;

    use crate::live::held::HostSlot;
    use crate::live::held::HostSpec;
    use crate::live::state::State;
    use crate::test_support::{FakeDaemon, local};

    fn refused_when_empty(next: Option<Connection>) -> daemonapi::Result<Connection> {
        next.ok_or_else(|| daemonapi::Error::Stream("no more connections".into()))
    }

    /// Hands out the scripted connections one per dial, then fails.
    struct Script(Mutex<VecDeque<Connection>>);

    impl Dial for Script {
        fn open(&self) -> Opening<'_> {
            let next = self.0.lock().ok().and_then(|mut queue| queue.pop_front());
            Box::pin(std::future::ready(refused_when_empty(next)))
        }
    }

    struct Peer {
        frames: mpsc::Sender<String>,
        requests: mpsc::UnboundedReceiver<String>,
    }

    fn pair() -> (Connection, Peer) {
        let (frames, incoming) = mpsc::channel(64);
        let (outgoing, requests) = mpsc::unbounded_channel();
        (Connection { incoming, outgoing }, Peer { frames, requests })
    }

    const READY: &str = r#"{"t":"ready","epoch":"e"}"#;
    const SYNCED: &str = r#"{"t":"synced","scope":"projects","unread":[]}"#;

    fn project(id: &str) -> String {
        format!(
            r#"{{"t":"entity","scope":"projects","kind":"project","id":"{id}","v":1,"data":{{"id":"{id}","name":"{id}"}}}}"#
        )
    }

    struct Rig {
        hub: Arc<Hub>,
        changed: Arc<Notify>,
        controls: mpsc::UnboundedSender<StreamControl>,
    }

    fn start(connections: Vec<Connection>) -> Rig {
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let client = daemonapi::Client::new(FakeDaemon::shared(), local());
        let spec = HostSpec {
            id: "local".into(),
            name: "L".into(),
            endpoint: None,
        };
        let hub = Hub::new(
            State::new(vec![HostSlot::new(spec, client.clone())]),
            Arc::new(move || wake.notify_one()),
        );
        let (controls, inbox) = mpsc::unbounded_channel();
        let stream = HostStream {
            hub: Arc::clone(&hub),
            host: "local".into(),
            dial: Arc::new(Script(Mutex::new(connections.into()))),
            client,
        };
        tokio::spawn(stream.run(inbox));
        Rig {
            hub,
            changed,
            controls,
        }
    }

    async fn wait_until(
        hub: &Hub,
        changed: &Notify,
        ready: impl Fn(&State) -> bool,
    ) {
        while !hub.read(&ready) {
            changed.notified().await;
        }
    }

    fn old_replaced_by_new(state: &State) -> bool {
        let projects = &state.hosts[0].store.projects;
        projects.get("new").is_some() && projects.get("old").is_none()
    }

    impl Rig {
        async fn until(
            &self,
            ready: impl Fn(&State) -> bool,
        ) {
            wait_until(&self.hub, &self.changed, ready).await;
        }
    }

    #[tokio::test(start_paused = true)]
    async fn ready_subscribes_and_the_snapshot_syncs_the_host() {
        let (connection, mut peer) = pair();
        let rig = start(vec![connection]);
        peer.frames.send(READY.into()).await.unwrap();
        let request = peer.requests.recv().await.unwrap();
        assert_eq!(request, r#"{"op":"sub","scope":"projects"}"#);
        peer.frames.send(project("p")).await.unwrap();
        peer.frames.send(SYNCED.into()).await.unwrap();
        rig.until(|state| state.hosts[0].store.synced() && state.hosts[0].link == LinkState::Live)
            .await;
        assert!(
            rig.hub
                .read(|state| state.hosts[0].store.projects.get("p").is_some())
        );
        rig.controls.send(StreamControl::Resubscribe).unwrap();
        assert_eq!(
            peer.requests.recv().await.unwrap(),
            r#"{"op":"sub","scope":"projects"}"#,
            "a resubscribe asks again"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn a_dropped_socket_marks_the_host_down_then_a_redial_replaces_the_snapshot() {
        let (first, mut one) = pair();
        let (second, mut two) = pair();
        let rig = start(vec![first, second]);
        one.frames.send(READY.into()).await.unwrap();
        one.requests.recv().await.unwrap();
        one.frames.send(project("old")).await.unwrap();
        one.frames.send(SYNCED.into()).await.unwrap();
        rig.until(|state| state.hosts[0].store.projects.get("old").is_some())
            .await;
        drop(one);
        rig.until(|state| state.hosts[0].link == LinkState::Down)
            .await;
        two.frames.send(READY.into()).await.unwrap();
        two.requests.recv().await.unwrap();
        two.frames.send(project("new")).await.unwrap();
        two.frames.send(SYNCED.into()).await.unwrap();
        rig.until(old_replaced_by_new).await;
    }

    #[derive(Clone, Copy, Debug)]
    enum Ending {
        NoConnection,
        ControlsDropped,
    }

    async fn no_connection_is_down() -> bool {
        let rig = start(Vec::new());
        rig.until(|state| state.hosts[0].link == LinkState::Down)
            .await;
        true
    }

    async fn dropped_controls_stop_the_stream() -> bool {
        let (connection, mut peer) = pair();
        let rig = start(vec![connection]);
        peer.frames.send(READY.into()).await.unwrap();
        peer.requests.recv().await.unwrap();
        drop(rig.controls);
        peer.requests.recv().await.is_none()
    }

    async fn each_holds(cases: [Ending; 2]) {
        for ending in cases {
            let held = match ending {
                Ending::NoConnection => no_connection_is_down().await,
                Ending::ControlsDropped => dropped_controls_stop_the_stream().await,
            };
            assert!(held, "{ending:?}");
        }
    }

    #[tokio::test(start_paused = true)]
    async fn a_stream_that_cannot_dial_is_down_and_one_without_controls_stops() {
        let cases = [Ending::NoConnection, Ending::ControlsDropped];
        each_holds(cases).await;
    }
}
