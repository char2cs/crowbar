pub(crate) mod adapters;
mod clock;

use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use http::StatusCode;
use protocol::{HostId, PaneId, RelayScope};
use tokio_util::sync::CancellationToken;

use crate::hosts::Target;
use crate::relay::connections::Book;
use crate::relay::connections::Guard;
use crate::relay::grants::OriginPolicy;
use crate::relay::grants::TokenBook;
use crate::relay::relay_config::RelayConfig;
use crate::relay::state::clock::Clock;
use crate::transport::Transport;

pub(crate) struct State {
    pub(crate) transport: Arc<Transport>,
    pub(crate) config: RelayConfig,
    pub(crate) policy: Arc<dyn OriginPolicy>,
    pub(crate) tokens: Mutex<TokenBook>,
    pub(crate) connections: Arc<Book>,
    pub(crate) port: u16,
    pub(crate) shutdown: CancellationToken,
    pub(crate) clock: Clock,
}

impl State {
    pub(crate) fn tokens(&self) -> MutexGuard<'_, TokenBook> {
        self.tokens.lock().unwrap_or_else(PoisonError::into_inner)
    }

    pub(crate) fn admit_stream(
        &self,
        token: &str,
        accept: impl FnOnce(&RelayScope) -> Result<(), StatusCode>,
    ) -> Result<(RelayScope, Guard), StatusCode> {
        let mut tokens = self.tokens();
        let scope = tokens
            .authorize(token, (self.clock)())
            .ok_or(StatusCode::UNAUTHORIZED)?;
        accept(&scope)?;
        let guard = self
            .connections
            .register(&scope, &self.shutdown)
            .ok_or(StatusCode::SERVICE_UNAVAILABLE)?;
        Ok((scope, guard))
    }

    pub(crate) fn revoke_pane(
        &self,
        pane: &PaneId,
    ) {
        let mut tokens = self.tokens();
        tokens.revoke_pane(pane);
        self.connections.close_pane(pane);
    }

    pub(crate) fn revoke_host(
        &self,
        host: &HostId,
    ) {
        let mut tokens = self.tokens();
        tokens.revoke_host(host);
        self.connections.close_host(host);
    }

    pub(crate) fn revoke_unregistered_hosts(&self) {
        let registry = self.transport.registry();
        let registered = |host: &HostId| match Target::of(host) {
            Target::Local => true,
            Target::Remote(id) => registry.get(&id).is_some(),
        };
        let mut tokens = self.tokens();
        tokens.retain_hosts(registered);
        self.connections.close_hosts_not_kept(registered);
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneOrigin;
    use tokio::time::Instant;

    use super::*;
    use crate::hosts::Endpoint;
    use crate::hosts::HostRegistry;
    use crate::relay::grants::ExactOrigins;
    use crate::relay::state::adapters::tokio_clock::tokio_now;
    use crate::transport::MemoryTokenStore;

    fn state_with(registry: HostRegistry) -> State {
        let transport = Transport::new(
            Arc::new(std::path::PathBuf::from("/nonexistent/cb-relay-state.sock")),
            Arc::new(registry),
            Arc::new(MemoryTokenStore::default()),
        );
        State {
            transport: Arc::new(transport),
            config: RelayConfig::default(),
            policy: Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}")),
            tokens: Mutex::new(TokenBook::new(std::time::Duration::from_secs(60))),
            connections: Book::new(4, 4),
            port: 1,
            shutdown: CancellationToken::new(),
            clock: Arc::new(tokio_now),
        }
    }

    fn scope(
        pane: &str,
        host: &str,
    ) -> RelayScope {
        RelayScope::new(
            HostId::new(host).unwrap(),
            PaneId::new(pane).unwrap(),
            PaneOrigin::Core,
        )
    }

    fn issue(
        state: &State,
        scope: RelayScope,
    ) -> String {
        state.tokens().issue(scope, Instant::now()).unwrap()
    }

    #[tokio::test]
    async fn a_pane_revoked_between_the_token_check_and_the_open_stream_leaves_no_stream() {
        let state = state_with(HostRegistry::in_memory());
        let token = issue(&state, scope("p1", "local"));
        let (_, guard) = state.admit_stream(&token, |_| Ok(())).unwrap();
        state.revoke_pane(&PaneId::new("p1").unwrap());
        assert!(
            guard.cancelled().is_cancelled(),
            "registered before the revoke, so cancelled by it"
        );
        assert_eq!(
            state.admit_stream(&token, |_| Ok(())).err(),
            Some(StatusCode::UNAUTHORIZED),
            "revoked before the check, so refused"
        );
    }

    #[tokio::test]
    async fn a_refusing_policy_registers_nothing() {
        let state = state_with(HostRegistry::in_memory());
        let token = issue(&state, scope("p1", "local"));
        assert_eq!(
            state
                .admit_stream(&token, |_| Err(StatusCode::FORBIDDEN))
                .err(),
            Some(StatusCode::FORBIDDEN)
        );
        assert!(
            state
                .connections
                .register(&scope("p1", "local"), &state.shutdown)
                .is_some(),
            "no slot was taken by the refused dial"
        );
    }

    type Revoke = fn(&State);

    const CASES: [Revoke; 2] = [
        |state| state.revoke_host(&HostId::new("gone").unwrap()),
        State::revoke_unregistered_hosts,
    ];

    fn assert_only_the_departed_host_is_revoked(revoke: Revoke) {
        let registry = HostRegistry::in_memory();
        registry
            .insert_blocking(
                HostId::new("kept").unwrap(),
                Endpoint::parse("10.0.0.2:1").unwrap(),
            )
            .unwrap();
        let state = state_with(registry);
        let kept = issue(&state, scope("p1", "kept"));
        let gone = issue(&state, scope("p2", "gone"));
        let local = issue(&state, scope("p3", "local"));
        let (_, kept_guard) = state.admit_stream(&kept, |_| Ok(())).unwrap();
        let (_, gone_guard) = state.admit_stream(&gone, |_| Ok(())).unwrap();
        let (_, local_guard) = state.admit_stream(&local, |_| Ok(())).unwrap();

        revoke(&state);

        assert!(gone_guard.cancelled().is_cancelled());
        assert!(!kept_guard.cancelled().is_cancelled());
        assert!(!local_guard.cancelled().is_cancelled());
        assert_eq!(
            state.admit_stream(&gone, |_| Ok(())).err(),
            Some(StatusCode::UNAUTHORIZED)
        );
        assert!(state.admit_stream(&kept, |_| Ok(())).is_ok());
        assert!(state.admit_stream(&local, |_| Ok(())).is_ok());
    }

    #[tokio::test]
    async fn a_host_that_leaves_loses_its_tokens_and_streams_but_neither_local_nor_listed_ones() {
        CASES
            .into_iter()
            .for_each(assert_only_the_departed_host_is_revoked);
    }
}
