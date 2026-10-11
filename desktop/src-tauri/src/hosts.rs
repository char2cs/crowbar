//! In-memory registry of where each remote host this session knows is
//! reachable. This is a RUNTIME cache the web layer populates at startup and
//! on every host add/remove/edit (its own persistence — likely
//! tauri-plugin-store — owns the durable copy; this registry only ever answers
//! "where do I dial host X right now"). Which certificate that dial trusts is
//! NOT here: pairing binds it to the host's token — see host_tokens.rs.

use std::collections::HashMap;
use std::sync::Mutex;

use crate::api_proxy::is_remote_host;

/// A host id is the authority of `crowbar://<hostId>/...`, so it must be one
/// the router sends to that host: never the local daemon's, and in a form no
/// URL parser rewrites.
pub fn validate_host_id(host_id: &str) -> Result<(), String> {
    let well_formed = host_id
        .bytes()
        .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-');
    if !well_formed {
        return Err(format!(
            "host id {host_id:?} may only hold lowercase letters, digits and hyphens"
        ));
    }
    if !is_remote_host(host_id) {
        return Err(format!("host id {host_id:?} names the local daemon"));
    }
    Ok(())
}

/// Host id → the `host:port` its daemon listens on.
#[derive(Default)]
pub struct HostRegistry(Mutex<HashMap<String, String>>);

impl HostRegistry {
    pub fn get(&self, host_id: &str) -> Option<String> {
        self.0.lock().unwrap().get(host_id).cloned()
    }

    pub fn insert(&self, host_id: String, endpoint: String) -> Result<(), String> {
        validate_host_id(&host_id)?;
        self.0.lock().unwrap().insert(host_id, endpoint);
        Ok(())
    }

    pub fn remove(&self, host_id: &str) {
        self.0.lock().unwrap().remove(host_id);
    }
}

#[tauri::command]
pub fn register_remote_host(
    host_id: String,
    endpoint: String,
    registry: tauri::State<'_, HostRegistry>,
) -> Result<(), String> {
    registry.insert(host_id, endpoint)
}

#[tauri::command]
pub fn unregister_remote_host(
    host_id: String,
    registry: tauri::State<'_, HostRegistry>,
    streams: tauri::State<'_, crate::ws_bridge::WsBridgeManager>,
) -> Result<(), String> {
    registry.remove(&host_id);
    streams.close_for_host(&host_id);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_then_get_roundtrips() {
        let reg = HostRegistry::default();
        reg.insert("host-1".into(), "1.2.3.4:3737".into()).unwrap();
        assert_eq!(reg.get("host-1").as_deref(), Some("1.2.3.4:3737"));
    }

    #[test]
    fn get_unknown_host_returns_none() {
        let reg = HostRegistry::default();
        assert!(reg.get("no-such-host").is_none());
    }

    #[test]
    fn remove_makes_it_unreachable() {
        // Review Focus: removing a host must make it immediately un-proxyable.
        let reg = HostRegistry::default();
        reg.insert("host-1".into(), "1.2.3.4:3737".into()).unwrap();
        reg.remove("host-1");
        assert!(reg.get("host-1").is_none());
    }

    #[test]
    fn a_host_id_the_router_would_not_send_to_that_host_is_refused() {
        let reg = HostRegistry::default();
        let refused = [
            "",
            "localhost",
            "Host-1",
            "host@1",
            "host:1",
            "host/1",
            "host.1",
            "host 1",
        ];
        for host_id in refused {
            let err = reg
                .insert(host_id.into(), "1.2.3.4:3737".into())
                .expect_err(host_id);
            assert!(err.contains("host id"), "{host_id:?}: {err}");
            assert!(reg.get(host_id).is_none(), "{host_id:?} was registered");
        }
        for host_id in ["host-1", "0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43"] {
            reg.insert(host_id.into(), "1.2.3.4:3737".into())
                .expect(host_id);
        }
    }
}
