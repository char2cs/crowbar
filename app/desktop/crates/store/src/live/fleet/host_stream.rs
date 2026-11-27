mod backoff;
mod ended;
mod file_change;
mod files_hold;
mod files_wire;
mod host_stream_event;
mod icon_ask;
mod icon_fetch;
mod step;
mod terminal_offer;

use std::sync::Arc;

use daemonapi::{Connection, Dial, Frame};
use serde_json::json;
use tokio::sync::mpsc;

use crate::journal_sink::emit;
use crate::live::OutcomeSink;
use crate::live::fleet::host_stream::backoff::Backoff;
use crate::live::fleet::host_stream::ended::Ended;
use crate::live::fleet::host_stream::files_hold::FilesHold;
use crate::live::fleet::host_stream::host_stream_event::HostStreamEvent;
use crate::live::fleet::host_stream::step::{Step, next_step};
use crate::live::fleet::host_stream::terminal_offer::read_terminal_support;
use crate::live::fleet::stream_control::StreamControl;
use crate::live::held::LinkState;
use crate::live::held::PROJECTS_SCOPE;
use crate::live::panel_loader::{changes_reload, followed_ask};
use crate::live::state::Hub;

const RESYNC: &str = "resync";

pub(crate) struct HostStream {
    pub(crate) hub: Arc<Hub>,
    pub(crate) host: String,
    pub(crate) dial: Arc<dyn Dial>,
    pub(crate) client: daemonapi::Client,
    pub(crate) outcome: OutcomeSink,
}

impl HostStream {
    pub(crate) async fn run(
        self,
        mut controls: mpsc::UnboundedReceiver<StreamControl>,
    ) {
        let mut backoff = Backoff::default();
        self.set_link(LinkState::Connecting);
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
        let ended = self.connect_once(controls, backoff).await;
        if matches!(ended, Ended::Stopped) {
            return ended;
        }
        self.hub.edit(|state| {
            state.lose_snapshot(&self.host);
            state.link(&self.host, LinkState::Down);
        });
        tokio::time::sleep(backoff.wait()).await;
        ended
    }

    async fn connect_once(
        &self,
        controls: &mut mpsc::UnboundedReceiver<StreamControl>,
        backoff: &mut Backoff,
    ) -> Ended {
        let error = match self.dial.open().await {
            Ok(connection) => return self.session(connection, controls, backoff).await,
            Err(error) => error,
        };
        emit(&HostStreamEvent::Unopened {
            host: &self.host,
            error: &error,
        });
        Ended::Dropped
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
        let mut files = None;
        loop {
            match next_step(&mut incoming, controls).await {
                Step::Frame(None) => return Ended::Dropped,
                Step::Frame(Some(text)) => self.on_text(&text, (&outgoing, &mut files), backoff),
                Step::Control(Some(control)) => self.steer(control, (&outgoing, &mut files)),
                Step::Control(None) => return Ended::Stopped,
            }
        }
    }

    fn steer(
        &self,
        control: StreamControl,
        (outgoing, files): Wire<'_>,
    ) {
        match control {
            StreamControl::Resubscribe => self.subscribe(outgoing),
            StreamControl::FollowFiles => FilesHold::sync_held(files, outgoing),
        }
    }

    fn on_text(
        &self,
        text: &str,
        wire: Wire<'_>,
        backoff: &mut Backoff,
    ) {
        let decoded = Frame::decode(text).inspect_err(|error| {
            tracing::warn!(host = %self.host, %error, "a stream frame was unreadable and skipped");
        });
        let Ok(frame) = decoded else {
            return;
        };
        if matches!(frame, Frame::Ready { .. }) {
            backoff.reset();
        }
        self.react(frame, wire);
    }

    fn react(
        &self,
        frame: Frame,
        (outgoing, files): Wire<'_>,
    ) {
        if files.iter().any(|held| held.hear(&frame, outgoing)) {
            return;
        }
        let host = self.host.as_str();
        match &frame {
            Frame::Ready { .. } => self.on_ready(frame, (outgoing, files)),
            Frame::Closed { scope, reason, .. } if scope == PROJECTS_SCOPE && reason == RESYNC => {
                self.subscribe(outgoing);
            },
            Frame::Closed {
                scope,
                reason,
                problem,
            } => emit(&HostStreamEvent::Closed {
                host,
                scope,
                reason,
                problem: problem.as_ref(),
            }),
            Frame::Error { problem } => emit(&HostStreamEvent::Refused { host, problem }),
            _ => {
                self.apply(frame);
                FilesHold::sync_held(files, outgoing);
            },
        }
    }

    fn on_ready(
        &self,
        frame: Frame,
        (outgoing, files): Wire<'_>,
    ) {
        self.set_link(LinkState::Live);
        self.apply(frame);
        self.subscribe(outgoing);
        let held = files.insert(FilesHold::new(
            &self.hub,
            &self.host,
            Arc::clone(&self.outcome),
        ));
        held.sync(outgoing);
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
        let handle = tokio::runtime::Handle::current();
        followed_ask::ask(&self.hub, (&handle, &self.outcome));
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

type Wire<'a> = (&'a mpsc::UnboundedSender<String>, &'a mut Option<FilesHold>);

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
        start_on(connections, FakeDaemon::shared())
    }

    fn start_on(
        connections: Vec<Connection>,
        daemon: Arc<FakeDaemon>,
    ) -> Rig {
        start_dialling(Arc::new(Script(Mutex::new(connections.into()))), daemon)
    }

    fn start_dialling(
        dial: Arc<dyn Dial>,
        daemon: Arc<FakeDaemon>,
    ) -> Rig {
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let client = daemonapi::Client::new(daemon, local());
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
            dial,
            client,
            outcome: Arc::new(|_| {}),
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

    const LISTING_DEADLINE: std::time::Duration = std::time::Duration::from_secs(5);
    const CHAT: &str = r#"{"t":"entity","scope":"projects","kind":"chat","id":"c","v":1,"data":{"id":"c","projectId":"p","workspaceId":"w"}}"#;

    #[tokio::test(start_paused = true)]
    async fn a_chat_followed_before_its_host_knew_it_is_listed_once_it_arrives() {
        let daemon = FakeDaemon::shared();
        daemon.ok("GET", "/v0/chats/c/files/tree?path=.", "[]");
        let (connection, mut peer) = pair();
        let rig = start_on(vec![connection], daemon);
        rig.hub.edit(|state| state.panels.follow(Some("c".into())));
        peer.frames.send(READY.into()).await.unwrap();
        peer.requests.recv().await.unwrap();
        peer.frames.send(CHAT.into()).await.unwrap();
        peer.frames.send(SYNCED.into()).await.unwrap();
        let listed = rig.until(|state| state.panels.listings.contains_key(""));
        tokio::time::timeout(LISTING_DEADLINE, listed)
            .await
            .expect("the root is listed");
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

    #[derive(Default)]
    struct Unreachable {
        hub: std::sync::OnceLock<Arc<Hub>>,
        seen: Mutex<Vec<LinkState>>,
        dialled: Notify,
    }

    impl Dial for Unreachable {
        fn open(&self) -> Opening<'_> {
            let link = self
                .hub
                .get()
                .map(|hub| hub.read(|state| state.hosts[0].link));
            self.seen.lock().unwrap().extend(link);
            self.dialled.notify_one();
            Box::pin(std::future::ready(refused_when_empty(None)))
        }
    }

    async fn links_at(
        dial: &Unreachable,
        dials: usize,
    ) -> Vec<LinkState> {
        while dial.seen.lock().unwrap().len() < dials {
            dial.dialled.notified().await;
        }
        dial.seen.lock().unwrap().clone()
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_stays_down_while_it_is_dialled_again() {
        let dial = Arc::new(Unreachable::default());
        let rig = start_dialling(Arc::clone(&dial) as Arc<dyn Dial>, FakeDaemon::shared());
        dial.hub.set(Arc::clone(&rig.hub)).ok();
        let seen = links_at(&dial, 3).await;
        let down = LinkState::Down;
        assert_eq!(seen, [LinkState::Connecting, down, down]);
    }
}
