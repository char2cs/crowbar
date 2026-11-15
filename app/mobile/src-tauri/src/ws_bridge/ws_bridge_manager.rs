use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use tokio::sync::mpsc;
use tokio_tungstenite::tungstenite::Message;

use super::connection::Connection;

pub(super) type Connections = Arc<Mutex<HashMap<String, Connection>>>;

#[derive(Default)]
pub(crate) struct WsBridgeManager {
    pub(super) connections: Connections,
    next_generation: AtomicU64,
}

pub(super) fn locked(connections: &Connections) -> MutexGuard<'_, HashMap<String, Connection>> {
    connections.lock().unwrap_or_else(PoisonError::into_inner)
}

pub(super) fn retire_generation(
    connections: &Connections,
    conn_id: &str,
    generation: u64,
) {
    let mut live = locked(connections);
    let current = live
        .get(conn_id)
        .is_some_and(|connection| connection.generation == generation);
    if current {
        live.remove(conn_id);
    }
}

impl WsBridgeManager {
    pub(super) fn sender(
        &self,
        conn_id: &str,
    ) -> Option<mpsc::UnboundedSender<Message>> {
        locked(&self.connections)
            .get(conn_id)
            .map(|connection| connection.tx.clone())
    }

    pub(crate) fn close_for_window(
        &self,
        label: &str,
    ) {
        locked(&self.connections).retain(|_, connection| connection.window_label != label);
    }

    pub(crate) fn close_for_host(
        &self,
        host_id: &str,
    ) {
        locked(&self.connections)
            .retain(|_, connection| connection.host_id.as_deref() != Some(host_id));
    }

    pub(super) fn retire(
        &self,
        conn_id: &str,
    ) {
        locked(&self.connections).remove(conn_id);
    }

    pub(super) fn claim_generation(&self) -> u64 {
        self.next_generation.fetch_add(1, Ordering::Relaxed)
    }

    pub(super) fn register(
        &self,
        conn_id: String,
        connection: Connection,
    ) {
        locked(&self.connections).insert(conn_id, connection);
    }

    pub(super) fn handle(&self) -> Connections {
        Arc::clone(&self.connections)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::sync::oneshot;

    fn connection(
        generation: u64,
        window: &str,
        host: Option<&str>,
    ) -> Connection {
        let (tx, _rx) = mpsc::unbounded_channel();
        let (cancel, _cancelled) = oneshot::channel();
        Connection {
            generation,
            window_label: window.to_owned(),
            host_id: host.map(str::to_owned),
            tx,
            _cancel: cancel,
        }
    }

    fn populated() -> WsBridgeManager {
        let manager = WsBridgeManager::default();
        manager.register("a".into(), connection(0, "main", None));
        manager.register("b".into(), connection(1, "w2", Some("h1")));
        manager.register("c".into(), connection(2, "main", Some("h2")));
        manager
    }

    fn remaining(manager: &WsBridgeManager) -> Vec<String> {
        let mut ids: Vec<String> = locked(&manager.connections).keys().cloned().collect();
        ids.sort();
        ids
    }

    fn close(
        manager: &WsBridgeManager,
        window: Option<&str>,
        host: Option<&str>,
    ) {
        if let Some(label) = window {
            manager.close_for_window(label);
        }
        if let Some(id) = host {
            manager.close_for_host(id);
        }
    }

    #[test]
    fn closing_by_window_or_host_retires_only_the_matching_connections() {
        type Case = (
            Option<&'static str>,
            Option<&'static str>,
            &'static [&'static str],
        );
        const CASES: &[Case] = &[
            (Some("main"), None, &["b"]),
            (Some("w2"), None, &["a", "c"]),
            (Some("nobody"), None, &["a", "b", "c"]),
            (None, Some("h1"), &["a", "c"]),
            (None, Some("h2"), &["a", "b"]),
            (None, Some("none"), &["a", "b", "c"]),
        ];
        for (window, host, expected) in CASES {
            let manager = populated();
            close(&manager, *window, *host);
            assert_eq!(remaining(&manager), *expected, "{window:?} {host:?}");
        }
    }

    #[test]
    fn a_stale_generation_leaves_a_reused_id_alone() {
        const CASES: &[(u64, &[&str])] = &[(1, &["a", "b", "c"]), (0, &["b", "c"])];
        for (generation, expected) in CASES {
            let manager = populated();
            retire_generation(&manager.handle(), "a", *generation);
            assert_eq!(remaining(&manager), *expected, "generation {generation}");
        }
    }

    #[test]
    fn only_a_registered_connection_has_a_sender() {
        let manager = populated();
        assert!(manager.sender("a").is_some(), "registered");
        manager.retire("a");
        assert!(manager.sender("a").is_none(), "retired");
        assert!(manager.sender("missing").is_none(), "never opened");
        assert_eq!(manager.claim_generation() + 1, manager.claim_generation());
    }
}
