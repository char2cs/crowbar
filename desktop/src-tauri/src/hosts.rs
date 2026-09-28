//! In-memory registry of remote hosts this session knows how to dial:
//! endpoint + pinned fingerprint. This is a RUNTIME cache the web layer
//! populates at startup and on every host add/remove/edit (its own
//! persistence — likely tauri-plugin-store — owns the durable copy; this
//! registry only ever answers "where and how do I dial host X right now").
//! Bearer tokens are NOT here — see host_tokens.rs.

use std::collections::HashMap;
use std::sync::Mutex;

#[derive(Clone, Debug)]
pub struct RemoteHost {
    pub endpoint: String,
    pub fingerprint: String,
}

#[derive(Default)]
pub struct HostRegistry(Mutex<HashMap<String, RemoteHost>>);

impl HostRegistry {
    pub fn get(&self, host_id: &str) -> Option<RemoteHost> {
        self.0.lock().unwrap().get(host_id).cloned()
    }

    pub fn insert(&self, host_id: String, host: RemoteHost) {
        self.0.lock().unwrap().insert(host_id, host);
    }

    pub fn remove(&self, host_id: &str) {
        self.0.lock().unwrap().remove(host_id);
    }
}

#[tauri::command]
pub fn register_remote_host(
    host_id: String,
    endpoint: String,
    fingerprint: String,
    registry: tauri::State<'_, HostRegistry>,
) -> Result<(), String> {
    registry.insert(
        host_id,
        RemoteHost {
            endpoint,
            fingerprint,
        },
    );
    Ok(())
}

#[tauri::command]
pub fn unregister_remote_host(
    host_id: String,
    registry: tauri::State<'_, HostRegistry>,
) -> Result<(), String> {
    registry.remove(&host_id);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn insert_then_get_roundtrips() {
        let reg = HostRegistry::default();
        reg.insert(
            "host-1".into(),
            RemoteHost {
                endpoint: "1.2.3.4:3737".into(),
                fingerprint: "SHA256:aaaa bbbb cccc dddd eeee ffff".into(),
            },
        );
        let got = reg.get("host-1").expect("expected host-1 to be registered");
        assert_eq!(got.endpoint, "1.2.3.4:3737");
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
        reg.insert(
            "host-1".into(),
            RemoteHost {
                endpoint: "1.2.3.4:3737".into(),
                fingerprint: "fp".into(),
            },
        );
        reg.remove("host-1");
        assert!(reg.get("host-1").is_none());
    }
}
