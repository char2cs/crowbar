use tauri::State;

use crate::host_error::HostError;
use crate::hosts::endpoint::parse_endpoint;
use crate::hosts::HostRegistry;
use crate::ws_bridge::WsBridgeManager;

#[tauri::command]
#[expect(
    clippy::needless_pass_by_value,
    reason = "tauri injects managed state by value"
)]
pub(crate) fn register_remote_host(
    host_id: String,
    endpoint: &str,
    registry: State<'_, HostRegistry>,
) -> Result<(), HostError> {
    registry.insert(host_id, parse_endpoint(endpoint)?)
}

#[tauri::command]
#[expect(
    clippy::needless_pass_by_value,
    reason = "tauri injects managed state by value"
)]
pub(crate) fn unregister_remote_host(
    host_id: &str,
    registry: State<'_, HostRegistry>,
    streams: State<'_, WsBridgeManager>,
) {
    registry.remove(host_id);
    streams.close_for_host(host_id);
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri::test::mock_app;
    use tauri::Manager;

    #[test]
    fn registering_makes_a_host_reachable_and_unregistering_removes_it() {
        let app = mock_app();
        app.manage(HostRegistry::default());
        app.manage(WsBridgeManager::default());
        register_remote_host("host-1".into(), "1.2.3.4:3737", app.state::<HostRegistry>()).unwrap();
        assert!(app.state::<HostRegistry>().get("host-1").is_some());
        unregister_remote_host(
            "host-1",
            app.state::<HostRegistry>(),
            app.state::<WsBridgeManager>(),
        );
        assert!(app.state::<HostRegistry>().get("host-1").is_none());
    }

    #[test]
    fn a_bad_id_or_endpoint_is_refused_and_registers_nothing() {
        const CASES: &[(&str, &str, &str)] = &[
            ("localhost", "1.2.3.4:3737", "host id"),
            ("Host-1", "1.2.3.4:3737", "host id"),
            ("host-1", "1.2.3.4", "is not a host:port address"),
        ];
        let app = mock_app();
        app.manage(HostRegistry::default());
        for (host_id, endpoint, reason) in CASES {
            let err =
                register_remote_host((*host_id).into(), endpoint, app.state::<HostRegistry>())
                    .unwrap_err();
            assert!(err.to_string().contains(reason), "{host_id}: {err}");
            assert!(app.state::<HostRegistry>().get(host_id).is_none());
        }
    }
}
