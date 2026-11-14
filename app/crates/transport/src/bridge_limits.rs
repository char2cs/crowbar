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
    pub(crate) fn of(config: &RelayConfig, upstream_idle: Option<Duration>) -> Self {
        Self {
            client_idle: config.client_idle,
            upstream_idle: upstream_idle.or(config.upstream_idle),
            write_timeout: config.write_timeout,
            close_grace: config.close_grace,
        }
    }
}

#[cfg(test)]
#[path = "bridge_limits_tests.rs"]
mod tests;
