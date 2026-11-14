use protocol::{HostId, RelayEndpoint};

const CHANNEL_SCRIPT: &str = include_str!("bootstrap.js");

/// The script every pane page runs before its own code: it defines `window.__CROWBAR__` with
/// the daemon base URL and relay endpoint, the `window.crowbar` message channel, and forwards
/// matching keybindings as `command.run`.
pub(crate) fn bootstrap_script(
    scheme: &str,
    host: &HostId,
    relay: Option<&RelayEndpoint>,
) -> String {
    let config = serde_json::json!({
        "api": format!("{scheme}://{host}"),
        "host": host.as_str(),
        "ws": relay,
    });
    format!("window.__CROWBAR__ = {config};\n{CHANNEL_SCRIPT}")
}

#[cfg(test)]
#[path = "bootstrap_tests.rs"]
mod tests;
