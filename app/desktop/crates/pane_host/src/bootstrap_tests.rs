use super::*;

#[test]
fn config_names_the_api_base_and_relay() {
    let host = HostId::new("local").unwrap();
    let relay = RelayEndpoint {
        url: "ws://127.0.0.1:9".into(),
        token: "t".into(),
    };
    let script = bootstrap_script("crowbar", &host, Some(&relay));
    assert!(script.starts_with(r#"window.__CROWBAR__ = {"api":"crowbar://local","host":"local","ws":{"token":"t","url":"ws://127.0.0.1:9"}};"#), "{script}");
    assert!(
        script.contains("__crowbarReceive"),
        "channel script is appended"
    );
}

#[test]
fn a_missing_relay_is_null() {
    let host = HostId::new("local").unwrap();
    assert!(
        bootstrap_script("crowbar", &host, None).contains(r#""ws":null"#),
        "null relay"
    );
}
