//! Lock order: `tokens` before `connections`, always. Admitting a stream (check the token, then
//! register it) and revoking (drop the tokens, then cancel the streams) each hold `tokens`
//! across both steps, so a stream is either refused or cancelled, never left open by a race.

use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use http::StatusCode;
use protocol::{HostId, PaneId, RelayScope};
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::connection_book::ConnectionBook;
use crate::connection_guard::ConnectionGuard;
use crate::host_target::HostTarget;
use crate::origin_policy::OriginPolicy;
use crate::relay_config::RelayConfig;
use crate::token_book::TokenBook;
use crate::transport::Transport;

pub(crate) struct RelayState {
    pub(crate) transport: Arc<Transport>,
    pub(crate) config: RelayConfig,
    pub(crate) policy: Arc<dyn OriginPolicy>,
    pub(crate) tokens: Mutex<TokenBook>,
    pub(crate) connections: Arc<ConnectionBook>,
    pub(crate) port: u16,
    pub(crate) shutdown: CancellationToken,
}

impl RelayState {
    pub(crate) fn tokens(&self) -> MutexGuard<'_, TokenBook> {
        self.tokens.lock().unwrap_or_else(PoisonError::into_inner)
    }

    pub(crate) fn admit(
        &self,
        token: &str,
        accept: impl FnOnce(&RelayScope) -> Result<(), StatusCode>,
    ) -> Result<(RelayScope, ConnectionGuard), StatusCode> {
        let mut tokens = self.tokens();
        let scope = tokens
            .authorize(token, Instant::now())
            .ok_or(StatusCode::UNAUTHORIZED)?;
        accept(&scope)?;
        let guard = self
            .connections
            .register(&scope, &self.shutdown)
            .ok_or(StatusCode::SERVICE_UNAVAILABLE)?;
        Ok((scope, guard))
    }

    pub(crate) fn revoke_pane(&self, pane: &PaneId) {
        let mut tokens = self.tokens();
        tokens.revoke_pane(pane);
        self.connections.close_pane(pane);
    }

    pub(crate) fn revoke_host(&self, host: &HostId) {
        let mut tokens = self.tokens();
        tokens.revoke_host(host);
        self.connections.close_host(host);
    }

    pub(crate) fn revoke_unregistered_hosts(&self) {
        let registry = self.transport.registry();
        let registered = |host: &HostId| match HostTarget::of(host) {
            HostTarget::Local => true,
            HostTarget::Remote(id) => registry.get(&id).is_some(),
        };
        let mut tokens = self.tokens();
        tokens.retain_hosts(registered);
        self.connections.close_hosts_not_kept(registered);
    }
}

#[cfg(test)]
#[path = "relay_state_tests.rs"]
mod tests;
