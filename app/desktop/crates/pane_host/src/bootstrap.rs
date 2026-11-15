use protocol::{HostId, RelayEndpoint};

const CHANNEL_SCRIPT: &str = include_str!("bootstrap.js");

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
