//! Following one host's `/v0/stream`: connect, subscribe, apply frames, reconnect.

use std::sync::Arc;

use serde_json::json;
use tokio::sync::mpsc;

use crate::backoff::Backoff;
use crate::connection::Connection;
use crate::daemon_client::DaemonClient;
use crate::dial::Dial;
use crate::ended::Ended;
use crate::entity_store::PROJECTS_SCOPE;
use crate::frame::{Frame, decode};
use crate::hub::Hub;
use crate::link_state::LinkState;
use crate::provider::Provider;
use crate::providers;
use crate::step::Step;
use crate::stream_control::StreamControl;

const RESYNC: &str = "resync";

pub(crate) struct HostStream {
    pub(crate) hub: Arc<Hub>,
    pub(crate) host: String,
    pub(crate) dial: Arc<dyn Dial>,
    pub(crate) client: DaemonClient,
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
        decode(text)
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
        self.hub.edit(|state| state.apply(&self.host, frame));
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

async fn read_terminal_support(
    hub: &Hub,
    host: &str,
    client: &DaemonClient,
) {
    let all = match providers::list(client).await {
        Ok(all) => all,
        Err(error) => {
            tracing::warn!(host, %error, "the host's agents could not be listed");
            return;
        },
    };
    let terminal = all.iter().any(Provider::offers_terminal);
    hub.edit(|state| state.set_terminal_threads(host, Some(terminal).into()));
}

async fn next_step(
    incoming: &mut mpsc::Receiver<String>,
    controls: &mut mpsc::UnboundedReceiver<StreamControl>,
) -> Step {
    tokio::select! {
        text = incoming.recv() => Step::Frame(text),
        control = controls.recv() => Step::Control(control),
    }
}

#[cfg(test)]
mod tests {
    use std::collections::VecDeque;
    use std::sync::{Arc, Mutex};

    use tokio::sync::Notify;

    use super::*;
    use crate::daemon_client::DaemonClient;
    use crate::dial::Opening;
    use crate::error::Error;
    use crate::host_slot::HostSlot;
    use crate::host_spec::HostSpec;
    use crate::state::State;
    use crate::test_support::{FakeDaemon, local};

    fn refused_when_empty(next: Option<Connection>) -> Result<Connection, Error> {
        next.ok_or_else(|| Error::Stream("no more connections".into()))
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
        let client = DaemonClient::new(FakeDaemon::shared(), local());
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

    #[tokio::test(start_paused = true)]
    async fn a_failed_dial_is_down_and_a_resubscribe_asks_again() {
        let rig = start(Vec::new());
        rig.until(|state| state.hosts[0].link == LinkState::Down)
            .await;
        let (connection, mut peer) = pair();
        let rig = start(vec![connection]);
        peer.frames.send(READY.into()).await.unwrap();
        peer.requests.recv().await.unwrap();
        rig.controls.send(StreamControl::Resubscribe).unwrap();
        assert_eq!(
            peer.requests.recv().await.unwrap(),
            r#"{"op":"sub","scope":"projects"}"#
        );
    }

    #[tokio::test(start_paused = true)]
    async fn closing_the_controls_stops_the_stream() {
        let (connection, mut peer) = pair();
        let rig = start(vec![connection]);
        peer.frames.send(READY.into()).await.unwrap();
        peer.requests.recv().await.unwrap();
        drop(rig.controls);
        assert!(
            peer.requests.recv().await.is_none(),
            "the stream dropped its end"
        );
    }
}
