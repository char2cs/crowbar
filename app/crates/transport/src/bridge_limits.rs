use std::time::Duration;

use crate::relay_config::RelayConfig;

#[derive(Debug, Clone, Copy)]
pub(crate) struct BridgeLimits {
    pub(crate) client_idle: Option<Duration>,
    pub(crate) upstream_idle: Option<Duration>,
    pub(crate) write_timeout: Duration,
    pub(crate) close_grace: Duration,
}

impl BridgeLimits {
    pub(crate) fn of(
        config: &RelayConfig,
        upstream_idle: Option<Duration>,
    ) -> Self {
        Self {
            client_idle: config.client_idle,
            upstream_idle: upstream_idle.or(config.upstream_idle),
            write_timeout: config.write_timeout,
            close_grace: config.close_grace,
        }
    }
}

#[cfg(test)]
mod tests {
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
}
