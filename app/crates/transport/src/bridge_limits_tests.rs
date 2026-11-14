use super::*;

#[test]
fn a_pane_chosen_upstream_idle_overrides_the_default_and_the_rest_follows_the_config() {
    let config = RelayConfig {
        upstream_idle: Some(Duration::from_secs(60)),
        client_idle: Some(Duration::from_secs(90)),
        ..RelayConfig::default()
    };
    let by_default = BridgeLimits::of(&config, None);
    assert_eq!(by_default.upstream_idle, Some(Duration::from_secs(60)));
    assert_eq!(by_default.client_idle, Some(Duration::from_secs(90)));
    let chosen = BridgeLimits::of(&config, Some(Duration::from_secs(5)));
    assert_eq!(chosen.upstream_idle, Some(Duration::from_secs(5)));
    assert_eq!(chosen.write_timeout, config.write_timeout);
    assert_eq!(chosen.close_grace, config.close_grace);
}
