use protocol::HostId;

const CHANNEL_SCRIPT: &str = include_str!("bootstrap.js");

pub(crate) fn bootstrap_script(
    scheme: &str,
    host: &HostId,
) -> String {
    let config = serde_json::json!({
        "api": format!("{scheme}://{host}"),
        "host": host.as_str(),
    });
    format!("window.__CROWBAR__ = {config};\n{CHANNEL_SCRIPT}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn config_names_the_api_base_and_host() {
        let host = HostId::new("local").unwrap();
        let script = bootstrap_script("crowbar", &host);
        assert!(
            script.starts_with(r#"window.__CROWBAR__ = {"api":"crowbar://local","host":"local"};"#),
            "{script}"
        );
        assert!(
            script.contains("__crowbarReceive"),
            "channel script is appended"
        );
    }

    #[test]
    fn messages_carry_the_page_generation_and_the_relay_is_fetched_on_demand() {
        let script = bootstrap_script("crowbar", &HostId::new("local").unwrap());
        assert!(script.contains("generation + ':' + text"), "envelope");
        assert!(script.contains("/__crowbar/relay"), "relay lookup");
    }
}
