//! Keeping [`SidebarStore`] in step with the daemon.
//!
//! The subscription-graph half of `web/src/components/app-sync-provider.tsx`.
//! One [`Scope`] is one open stream: seeded with a GET, then streamed over the
//! same path, because the daemon dual-serves both.
//!
//! # Seed before stream, and both on one task
//!
//! `subscribeEntityStream` registers the WS subscription **synchronously**
//! before its seed GET resolves, so no frame is dropped in between; the frames
//! that arrive during the seed are applied after it. Here the subscription is
//! opened first and its frames buffer in the channel while the seed runs on
//! the same task, which gets the same ordering without a queue of its own.
//!
//! # Tearing down before seeding the replacement
//!
//! On a project switch the previous project's streams are dropped **first**,
//! and its cached rows go with them, so the old project's repos leave the
//! screen at once rather than lingering until the new seed lands. React does
//! this in the same order and the order is the point.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

use crowbar_client::stream::{Frame, Subscription, decode_frame};
use crowbar_client::transport::get_json;
use crowbar_core::proto::api_v0_dto::{ProjectDTO, RepoDTO, WorkspaceDTO};
use crowbar_core::sidebar::cache::{Scope, Seed};
use gpui::{App, AsyncApp, Entity, Task, WeakEntity};

use crate::sidebar::{Decoded, SidebarStore};

/// How long a seed GET may take before it is abandoned.
///
/// Generous: the daemon builds this list off its own view store, which is the
/// resource every observed production wedge pinned, and a seed that times out
/// early turns a slow daemon into an empty sidebar. A reconnect reseeds
/// anyway, so the cost of waiting is bounded.
const SEED_TIMEOUT: Duration = Duration::from_secs(15);

/// The open streams, and the store they feed.
pub struct DaemonSync {
    socket: PathBuf,
    store: WeakEntity<SidebarStore>,
    open: HashMap<String, OpenScope>,
}

/// One live subscription and the task draining it. Dropping this closes the
/// socket, joins the reader thread and cancels the task.
struct OpenScope {
    _subscription: Subscription,
    _pump: Task<()>,
}

impl DaemonSync {
    /// Feed `store` from the daemon at `socket`.
    #[must_use]
    pub fn new(socket: &Path, store: &Entity<SidebarStore>) -> Self {
        Self {
            socket: socket.to_path_buf(),
            store: store.downgrade(),
            open: HashMap::new(),
        }
    }

    /// Which scopes are currently open. Ordered, so a test asserts on a set
    /// rather than on hash order.
    #[must_use]
    pub fn open_paths(&self) -> Vec<String> {
        let mut paths: Vec<String> = self.open.keys().cloned().collect();
        paths.sort();
        paths
    }

    /// Open `scope` if it is not already open.
    pub fn open(&mut self, scope: Scope, cx: &mut App) {
        let path = scope.path();
        if self.open.contains_key(&path) {
            return;
        }

        let subscription = Subscription::open(&self.socket, &path);
        let frames = subscription.frames();
        let socket = self.socket.clone();
        let store = self.store.clone();
        let stream_path = path.clone();

        let pump = cx.spawn(async move |cx: &mut AsyncApp| {
            reseed(&socket, &scope, &store, cx).await;

            while let Ok(frame) = frames.recv().await {
                match frame {
                    Frame::Data(text) => {
                        // Take everything the stream already had ready, so one
                        // poll's burst is one rebuild rather than one per row.
                        let mut batch = Vec::from([text]);
                        while let Ok(Frame::Data(more)) = frames.try_recv() {
                            batch.push(more);
                        }
                        let decoded: Vec<Decoded> =
                            batch.iter().filter_map(|raw| decode(&scope, raw)).collect();
                        if store
                            .update(cx, |store, cx| store.apply_batch(decoded, cx))
                            .is_err()
                        {
                            return;
                        }
                    }
                    Frame::Connected => {
                        if store
                            .update(cx, |store, cx| store.note_connected(&stream_path, cx))
                            .is_err()
                        {
                            return;
                        }
                    }
                    Frame::Disconnected(reason) => {
                        if store
                            .update(cx, |store, cx| {
                                store.note_disconnected(&stream_path, reason, cx);
                            })
                            .is_err()
                        {
                            return;
                        }
                    }
                    Frame::Reconnected => reseed(&socket, &scope, &store, cx).await,
                }
            }
        });

        self.open.insert(
            path,
            OpenScope {
                _subscription: subscription,
                _pump: pump,
            },
        );
    }

    /// Close `scope`, if open.
    pub fn close(&mut self, scope: &Scope, cx: &mut App) {
        let path = scope.path();
        if self.open.remove(&path).is_some() {
            let _ = self
                .store
                .update(cx, |store, cx| store.forget_channel(&path, cx));
        }
    }

    /// Close every scope whose path is not in `keep`.
    ///
    /// Used on a project switch: the previous project's repo and workspace
    /// streams go before the new project's are opened.
    pub fn retain(&mut self, keep: &[String], cx: &mut App) {
        let closing: Vec<String> = self
            .open
            .keys()
            .filter(|path| !keep.contains(path))
            .cloned()
            .collect();
        for path in closing {
            self.open.remove(&path);
            let _ = self
                .store
                .update(cx, |store, cx| store.forget_channel(&path, cx));
        }
    }
}

/// Run a seed for `scope` and apply it, unless a newer seed superseded it
/// while the GET was in flight.
async fn reseed(socket: &Path, scope: &Scope, store: &WeakEntity<SidebarStore>, cx: &mut AsyncApp) {
    let Ok(generation) = store.update(cx, |store, _| store.begin_reseed()) else {
        return;
    };

    let socket = socket.to_path_buf();
    let path = scope.path();
    let scope_for_fetch = scope.clone();
    // The GET is blocking — `reqwest` is pinned that way because gpui runs no
    // tokio reactor — so it goes to a background thread rather than stalling
    // the foreground executor for as long as the daemon takes.
    let seed = cx
        .background_executor()
        .spawn(async move { fetch_seed(&socket, &scope_for_fetch, &path) })
        .await;

    let Some(seed) = seed else {
        // A failed seed leaves the cache exactly as it was. The next
        // reconnect reseeds; there is nothing useful to show for it here, and
        // the stream's own `Disconnected` frame already drives the indicator.
        return;
    };

    let _ = store.update(cx, |store, cx| {
        store.apply_seed(scope, generation, seed, cx);
    });
}

/// The blocking half of a seed.
fn fetch_seed(socket: &Path, scope: &Scope, path: &str) -> Option<Seed> {
    match scope {
        Scope::Projects => get_json::<Vec<ProjectDTO>>(socket, path, SEED_TIMEOUT)
            .ok()
            .map(Seed::Projects),
        Scope::Repos { .. } => get_json::<Vec<RepoDTO>>(socket, path, SEED_TIMEOUT)
            .ok()
            .map(Seed::Repos),
        Scope::Workspaces { .. } => get_json::<Vec<WorkspaceDTO>>(socket, path, SEED_TIMEOUT)
            .ok()
            .map(Seed::Workspaces),
    }
}

/// Decode one stream frame into the DTO its scope carries.
///
/// A frame that does not decode is dropped rather than escalated: the React
/// stream does the same (`typeof frame.id !== 'string'` and it is ignored),
/// and a single malformed record must not take down a live subscription.
fn decode(scope: &Scope, raw: &str) -> Option<Decoded> {
    match scope {
        Scope::Projects => decode_frame(raw).map(Decoded::Project),
        Scope::Repos { .. } => decode_frame(raw).map(Decoded::Repo),
        Scope::Workspaces { .. } => decode_frame(raw).map(Decoded::Workspace),
    }
}

#[cfg(test)]
mod tests {
    use std::io::{BufRead as _, BufReader, Write as _};
    use std::os::unix::net::{UnixListener, UnixStream};
    use std::path::PathBuf;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::thread::JoinHandle;

    use crowbar_core::sidebar::cache::Scope;
    use gpui::TestAppContext;
    use tungstenite::Message;
    use tungstenite::handshake::derive_accept_key;
    use tungstenite::protocol::{Role, WebSocket};

    use super::DaemonSync;
    use crate::sidebar::SidebarStore;

    /// A fake daemon on a unix socket that **dual-serves**: a plain GET on a
    /// path, and a WebSocket upgrade on the same path.
    ///
    /// That duality is the shape the sync engine is built around — seed with
    /// the GET, stream the same path — and it is the one thing a stub cannot
    /// fake, because the client decides which it is doing by the request it
    /// sends, not by the URL it was handed.
    struct FakeDaemon {
        dir: PathBuf,
        socket: PathBuf,
        stop: Arc<AtomicBool>,
        server: Option<JoinHandle<()>>,
    }

    impl FakeDaemon {
        /// `seed_body` is the envelope the GET answers with; `push` is sent as
        /// a stream frame once the upgrade completes.
        fn serving(name: &str, seed_body: String, push: Option<String>) -> Self {
            let dir = std::env::temp_dir().join(format!(
                "crowbar-sync-{name}-{}-{:?}",
                std::process::id(),
                std::thread::current().id(),
            ));
            let _ = std::fs::remove_dir_all(&dir);
            std::fs::create_dir_all(&dir).expect("temp dir");
            let socket = dir.join("d.sock");
            let listener = UnixListener::bind(&socket).expect("bind");
            let stop = Arc::new(AtomicBool::new(false));

            // A connection per thread, because the real daemon serves them
            // concurrently and the sync engine relies on it: the subscription
            // is opened before the seed GET, so a fixture that answered
            // serially would hold the upgrade open and starve the seed behind
            // it. That is not a test artefact — it is the ordering the engine
            // is built on, and a serial fixture silently tests the wrong app.
            let server = std::thread::spawn({
                let stop = Arc::clone(&stop);
                move || {
                    let mut workers: Vec<JoinHandle<()>> = Vec::new();
                    while let Ok((stream, _)) = listener.accept() {
                        if stop.load(Ordering::SeqCst) {
                            break;
                        }
                        let seed_body = seed_body.clone();
                        let push = push.clone();
                        workers.push(std::thread::spawn(move || {
                            Self::answer(stream, &seed_body, push.as_deref());
                        }));
                    }
                    for worker in workers {
                        let _ = worker.join();
                    }
                }
            });

            Self {
                dir,
                socket,
                stop,
                server: Some(server),
            }
        }

        /// Read the request head, then branch on whether it asked to upgrade.
        fn answer(stream: UnixStream, seed_body: &str, push: Option<&str>) {
            let Ok(clone) = stream.try_clone() else {
                return;
            };
            let mut reader = BufReader::new(clone);
            let mut key = None;
            let mut upgrade = false;
            let mut line = String::new();
            while reader.read_line(&mut line).unwrap_or(0) > 0 {
                if line == "\r\n" || line == "\n" {
                    break;
                }
                let lower = line.to_ascii_lowercase();
                if lower.starts_with("sec-websocket-key:") {
                    key = line.split_once(':').map(|(_, v)| v.trim().to_string());
                }
                if lower.starts_with("upgrade:") && lower.contains("websocket") {
                    upgrade = true;
                }
                line.clear();
            }

            let mut out = &stream;
            if upgrade {
                let Some(key) = key else { return };
                // The 101 is written by hand rather than through
                // `tungstenite::accept`, which would want to read the request
                // head this fixture has already consumed to make the decision.
                let accept = derive_accept_key(key.as_bytes());
                let _ = out.write_all(
                    format!(
                        "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: {accept}\r\n\r\n"
                    )
                    .as_bytes(),
                );
                let _ = out.flush();
                let mut ws = WebSocket::from_raw_socket(stream, Role::Server, None);
                if let Some(push) = push {
                    let _ = ws.send(Message::text(push.to_string()));
                    let _ = ws.flush();
                }
                // Held open until the client tears it down.
                let _ = ws.read();
                return;
            }

            let _ = out.write_all(
                format!(
                    "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{seed_body}",
                    seed_body.len(),
                )
                .as_bytes(),
            );
            let _ = out.flush();
        }
    }

    impl Drop for FakeDaemon {
        fn drop(&mut self) {
            self.stop.store(true, Ordering::SeqCst);
            let _ = UnixStream::connect(&self.socket);
            if let Some(server) = self.server.take() {
                let _ = server.join();
            }
            let _ = std::fs::remove_dir_all(&self.dir);
        }
    }

    fn repos_scope() -> Scope {
        Scope::Repos {
            project_id: "p1".to_string(),
        }
    }

    const REPO_ROW: &str = r#"{"id":"r1","projectId":"p1","name":"repo-one","path":"/r/1","defaultBranch":"main","avatarLabel":"R","avatarColor":"avatar-slate"}"#;
    const LIVE_ROW: &str = r#"{"id":"r-live","projectId":"p1","name":"streamed-in","path":"/r/live","defaultBranch":"main","avatarLabel":"S","avatarColor":"avatar-rose"}"#;

    /// **The whole pipe.** A real unix socket, a real envelope over real HTTP,
    /// a real RFC 6455 upgrade on the same path, and the row arriving in the
    /// store the sidebar renders from.
    #[gpui::test]
    fn a_seed_reaches_the_store(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let daemon = FakeDaemon::serving(
            "seed",
            format!(r#"{{"success":true,"data":[{REPO_ROW}]}}"#),
            None,
        );

        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        cx.update(|cx| {
            store.update(cx, |store, cx| {
                store.set_active_project(Some("p1".to_string()), cx);
            });
        });

        let mut sync = cx.update(|_cx| DaemonSync::new(&daemon.socket, &store));
        cx.update(|cx| sync.open(repos_scope(), cx));
        cx.run_until_parked();

        store.read_with(cx, |store, _| {
            assert_eq!(store.repos().len(), 1, "the seed landed");
            assert_eq!(store.repos()[0].name, "repo-one");
        });
    }

    /// A pushed frame on the upgraded path updates the same store the seed
    /// filled — the thing a seeded-only sidebar cannot do.
    #[gpui::test]
    fn a_streamed_frame_reaches_the_store(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let daemon = FakeDaemon::serving(
            "push",
            r#"{"success":true,"data":[]}"#.to_string(),
            Some(LIVE_ROW.to_string()),
        );

        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        cx.update(|cx| {
            store.update(cx, |store, cx| {
                store.set_active_project(Some("p1".to_string()), cx);
            });
        });

        let mut sync = cx.update(|_cx| DaemonSync::new(&daemon.socket, &store));
        cx.update(|cx| sync.open(repos_scope(), cx));
        cx.run_until_parked();

        store.read_with(cx, |store, _| {
            assert_eq!(store.repos().len(), 1, "the pushed row is rendered");
            assert_eq!(store.repos()[0].name, "streamed-in");
            assert_eq!(
                store.connection(),
                crate::Connection::Live,
                "the open is reported to the indicator"
            );
        });
    }

    #[gpui::test]
    fn opening_the_same_scope_twice_opens_one_stream(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let daemon =
            FakeDaemon::serving("dedupe", r#"{"success":true,"data":[]}"#.to_string(), None);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        let mut sync = cx.update(|_cx| DaemonSync::new(&daemon.socket, &store));

        cx.update(|cx| {
            sync.open(repos_scope(), cx);
            sync.open(repos_scope(), cx);
        });
        cx.run_until_parked();

        assert_eq!(sync.open_paths(), vec!["/v0/projects/p1/repos".to_string()]);
    }

    /// On a project switch the previous project's streams go first, and a
    /// closed stream must stop holding the connection indicator red.
    #[gpui::test]
    fn retaining_closes_the_scopes_left_out(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let daemon =
            FakeDaemon::serving("retain", r#"{"success":true,"data":[]}"#.to_string(), None);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        let mut sync = cx.update(|_cx| DaemonSync::new(&daemon.socket, &store));

        cx.update(|cx| {
            sync.open(Scope::Projects, cx);
            sync.open(repos_scope(), cx);
        });
        cx.run_until_parked();
        assert_eq!(sync.open_paths().len(), 2);

        cx.update(|cx| sync.retain(&["/v0/projects".to_string()], cx));
        cx.run_until_parked();

        assert_eq!(sync.open_paths(), vec!["/v0/projects".to_string()]);
    }

    #[gpui::test]
    fn closing_a_scope_that_is_not_open_is_a_no_op(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let daemon =
            FakeDaemon::serving("close", r#"{"success":true,"data":[]}"#.to_string(), None);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        let mut sync = cx.update(|_cx| DaemonSync::new(&daemon.socket, &store));

        cx.update(|cx| sync.close(&repos_scope(), cx));
        assert!(sync.open_paths().is_empty());
    }

    /// A daemon that is down leaves the cache exactly as it was rather than
    /// clearing the sidebar — the stream keeps retrying behind it.
    #[gpui::test]
    fn a_dead_daemon_leaves_the_store_intact(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let missing = std::env::temp_dir().join("crowbar-sync-no-daemon.sock");
        let _ = std::fs::remove_file(&missing);

        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));
        let mut sync = cx.update(|_cx| DaemonSync::new(&missing, &store));
        cx.update(|cx| sync.open(repos_scope(), cx));
        cx.run_until_parked();

        store.read_with(cx, |store, _| {
            assert!(store.repos().is_empty());
            assert!(matches!(store.connection(), crate::Connection::Down(_)));
        });
    }
}
