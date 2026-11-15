use std::sync::Arc;
use std::time::Duration;

use http::{HeaderValue, StatusCode, header};
use protocol::RelayScope;
use tokio::sync::oneshot;
use tokio_tungstenite::tungstenite::handshake::server::{
    Callback, ErrorResponse, Request, Response,
};

use crate::access_scope;
use crate::admission::Admission;
use crate::relay_protocol::{IDLE_SUBPROTOCOL_PREFIX, RELAY_SUBPROTOCOL, TOKEN_SUBPROTOCOL_PREFIX};
use crate::relay_state::RelayState;

const MIN_IDLE_SECONDS: u64 = 5;
const MAX_IDLE_SECONDS: u64 = 3600;

pub(crate) struct Gate {
    state: Arc<RelayState>,
    admitted: oneshot::Sender<Admission>,
}

impl Gate {
    pub(crate) fn new(
        state: Arc<RelayState>,
        admitted: oneshot::Sender<Admission>,
    ) -> Self {
        Self { state, admitted }
    }
}

impl Callback for Gate {
    fn on_request(
        self,
        request: &Request,
        mut response: Response,
    ) -> Result<Response, ErrorResponse> {
        let admission = inspect(&self.state, request).map_err(refusal)?;
        self.admitted
            .send(admission)
            .map_err(|_| refusal(StatusCode::SERVICE_UNAVAILABLE))?;
        response.headers_mut().insert(
            header::SEC_WEBSOCKET_PROTOCOL,
            HeaderValue::from_static(RELAY_SUBPROTOCOL),
        );
        Ok(response)
    }
}

fn refusal(status: StatusCode) -> ErrorResponse {
    let mut response = ErrorResponse::new(None);
    *response.status_mut() = status;
    response
}

fn inspect(
    state: &RelayState,
    request: &Request,
) -> Result<Admission, StatusCode> {
    check_host(request, state.port)?;
    let origin = single_origin(request)?;
    let offered = offered_protocols(request);
    if !offered.iter().any(|p| p == RELAY_SUBPROTOCOL) {
        return Err(StatusCode::BAD_REQUEST);
    }
    let token = offered
        .iter()
        .find_map(|p| p.strip_prefix(TOKEN_SUBPROTOCOL_PREFIX))
        .ok_or(StatusCode::UNAUTHORIZED)?;
    let upstream_idle = requested_idle(&offered)?;
    let path_and_query = request.uri().path_and_query().map_or("/", |pq| pq.as_str());
    let (scope, guard) = state.admit(token, |scope| {
        permitted(state, scope, origin, path_and_query)
    })?;
    Ok(Admission {
        scope,
        path_and_query: path_and_query.to_owned(),
        upstream_idle,
        guard,
    })
}

fn permitted(
    state: &RelayState,
    scope: &RelayScope,
    origin: &str,
    path_and_query: &str,
) -> Result<(), StatusCode> {
    if !state.policy.allows(scope, origin) {
        return Err(StatusCode::FORBIDDEN);
    }
    access_scope::check(scope.origin(), path_and_query).map_err(|_| StatusCode::FORBIDDEN)
}

fn requested_idle(offered: &[String]) -> Result<Option<Duration>, StatusCode> {
    let Some(text) = offered
        .iter()
        .find_map(|p| p.strip_prefix(IDLE_SUBPROTOCOL_PREFIX))
    else {
        return Ok(None);
    };
    let seconds: u64 = text.parse().map_err(|_| StatusCode::BAD_REQUEST)?;
    let clamped = seconds.clamp(MIN_IDLE_SECONDS, MAX_IDLE_SECONDS);
    Ok(Some(Duration::from_secs(clamped)))
}

fn check_host(
    request: &Request,
    port: u16,
) -> Result<(), StatusCode> {
    let expected = format!("127.0.0.1:{port}");
    let host = request
        .headers()
        .get(header::HOST)
        .and_then(|v| v.to_str().ok());
    if host == Some(expected.as_str()) {
        return Ok(());
    }
    Err(StatusCode::FORBIDDEN)
}

fn single_origin(request: &Request) -> Result<&str, StatusCode> {
    let mut origins = request.headers().get_all(header::ORIGIN).iter();
    match (origins.next(), origins.next()) {
        (Some(origin), None) => origin.to_str().map_err(|_| StatusCode::FORBIDDEN),
        _ => Err(StatusCode::FORBIDDEN),
    }
}

fn offered_protocols(request: &Request) -> Vec<String> {
    request
        .headers()
        .get_all(header::SEC_WEBSOCKET_PROTOCOL)
        .iter()
        .filter_map(|value| value.to_str().ok())
        .flat_map(|list| list.split(','))
        .map(|protocol| protocol.trim().to_owned())
        .collect()
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;
    use std::time::Duration;

    use protocol::{ExtensionId, HostId, PaneId, PaneOrigin, RelayScope};

    use super::*;
    use crate::connection_book::ConnectionBook;
    use crate::exact_origins::ExactOrigins;
    use crate::host_registry::HostRegistry;
    use crate::memory_token_store::MemoryTokenStore;
    use crate::relay_config::RelayConfig;
    use crate::token_book::TokenBook;
    use crate::transport::Transport;
    use std::sync::Arc;
    use tokio::time::Instant;
    use tokio_util::sync::CancellationToken;

    const PORT: u16 = 4242;
    const ORIGIN: &str = "crowbar://app";

    fn state() -> Arc<RelayState> {
        let transport = Transport::new(
            Arc::new(std::env::temp_dir().join("cb-gate.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        );
        Arc::new(RelayState {
            transport: Arc::new(transport),
            config: RelayConfig::default(),
            policy: Arc::new(ExactOrigins::new(
                vec![ORIGIN.to_owned()],
                "crowbar-ext://{extension}",
            )),
            tokens: Mutex::new(TokenBook::new(Duration::from_secs(60))),
            connections: ConnectionBook::new(4, 4),
            port: PORT,
            shutdown: CancellationToken::new(),
        })
    }

    fn issue(
        state: &RelayState,
        origin: PaneOrigin,
    ) -> String {
        let scope = RelayScope::new(
            HostId::new("local").unwrap(),
            PaneId::new("p1").unwrap(),
            origin,
        );
        state.tokens().issue(scope, Instant::now()).unwrap()
    }

    fn acme() -> PaneOrigin {
        PaneOrigin::Extension(ExtensionId::new("acme").unwrap())
    }

    struct Dial<'a> {
        host: Option<String>,
        origins: Vec<&'a str>,
        protocols: Vec<String>,
        target: &'a str,
    }

    impl Dial<'_> {
        fn good(token: &str) -> Self {
            Self {
                host: Some(format!("127.0.0.1:{PORT}")),
                origins: vec![ORIGIN],
                protocols: vec![
                    RELAY_SUBPROTOCOL.into(),
                    format!("{TOKEN_SUBPROTOCOL_PREFIX}{token}"),
                ],
                target: "/v0/terminals/t1/stream",
            }
        }

        fn request(&self) -> Request {
            let mut builder = http::Request::builder().uri(self.target);
            if let Some(host) = &self.host {
                builder = builder.header(header::HOST, host);
            }
            for origin in &self.origins {
                builder = builder.header(header::ORIGIN, *origin);
            }
            for protocol in &self.protocols {
                builder = builder.header(header::SEC_WEBSOCKET_PROTOCOL, protocol);
            }
            builder.body(()).unwrap()
        }
    }

    #[tokio::test]
    async fn a_well_formed_dial_is_admitted_with_its_scope_and_target() {
        let state = state();
        let token = issue(&state, PaneOrigin::Core);
        let admission = inspect(&state, &Dial::good(&token).request()).unwrap();
        assert_eq!(admission.path_and_query, "/v0/terminals/t1/stream");
        assert_eq!(admission.scope.pane().as_str(), "p1");
    }

    #[tokio::test]
    async fn each_check_refuses_on_its_own() {
        let state = state();
        let token = issue(&state, PaneOrigin::Core);
        let refused = |dial: Dial<'_>| inspect(&state, &dial.request()).err();

        let mut wrong_host = Dial::good(&token);
        wrong_host.host = Some("evil.example:4242".into());
        assert_eq!(refused(wrong_host), Some(StatusCode::FORBIDDEN));

        let mut no_host = Dial::good(&token);
        no_host.host = None;
        assert_eq!(refused(no_host), Some(StatusCode::FORBIDDEN));

        let mut no_origin = Dial::good(&token);
        no_origin.origins = vec![];
        assert_eq!(refused(no_origin), Some(StatusCode::FORBIDDEN));

        let mut two_origins = Dial::good(&token);
        two_origins.origins = vec![ORIGIN, ORIGIN];
        assert_eq!(refused(two_origins), Some(StatusCode::FORBIDDEN));

        let mut foreign_origin = Dial::good(&token);
        foreign_origin.origins = vec!["https://evil.example"];
        assert_eq!(refused(foreign_origin), Some(StatusCode::FORBIDDEN));

        let mut no_relay_protocol = Dial::good(&token);
        no_relay_protocol.protocols.remove(0);
        assert_eq!(refused(no_relay_protocol), Some(StatusCode::BAD_REQUEST));

        let mut no_token = Dial::good(&token);
        no_token.protocols.truncate(1);
        assert_eq!(refused(no_token), Some(StatusCode::UNAUTHORIZED));

        assert_eq!(
            refused(Dial::good("not-a-token")),
            Some(StatusCode::UNAUTHORIZED)
        );
    }

    #[tokio::test]
    async fn an_extension_token_reaches_only_its_own_routes() {
        let state = state();
        let token = issue(&state, acme());
        let mut dial = Dial::good(&token);
        dial.origins = vec!["crowbar-ext://acme"];
        dial.target = "/v0/ext/acme/stream";
        assert!(inspect(&state, &dial.request()).is_ok());
        dial.target = "/v0/terminals/t1/stream";
        assert_eq!(
            inspect(&state, &dial.request()).err(),
            Some(StatusCode::FORBIDDEN)
        );
        dial.target = "/v0/ext/other/stream";
        assert_eq!(
            inspect(&state, &dial.request()).err(),
            Some(StatusCode::FORBIDDEN)
        );
    }

    #[test]
    fn a_comma_separated_protocol_list_is_split_and_trimmed() {
        let request = http::Request::builder()
            .header(header::SEC_WEBSOCKET_PROTOCOL, "a, b")
            .header(header::SEC_WEBSOCKET_PROTOCOL, "c")
            .body(())
            .unwrap();
        assert_eq!(offered_protocols(&request), ["a", "b", "c"]);
    }

    #[tokio::test]
    async fn admission_registers_the_stream_so_a_revoke_right_after_cancels_it() {
        let state = state();
        let token = issue(&state, PaneOrigin::Core);
        let admission = inspect(&state, &Dial::good(&token).request()).unwrap();
        assert!(!admission.guard.cancelled().is_cancelled());
        state.revoke_pane(&PaneId::new("p1").unwrap());
        assert!(admission.guard.cancelled().is_cancelled());
        assert_eq!(
            inspect(&state, &Dial::good(&token).request()).err(),
            Some(StatusCode::UNAUTHORIZED),
            "the token is gone for the next dial"
        );
    }

    #[tokio::test]
    async fn a_full_relay_refuses_before_the_upgrade_with_503() {
        let state = state();
        let token = issue(&state, PaneOrigin::Core);
        let held: Vec<_> = (0..4)
            .map(|_| inspect(&state, &Dial::good(&token).request()).unwrap())
            .collect();
        assert_eq!(
            inspect(&state, &Dial::good(&token).request()).err(),
            Some(StatusCode::SERVICE_UNAVAILABLE)
        );
        drop(held);
        assert!(inspect(&state, &Dial::good(&token).request()).is_ok());
    }

    #[tokio::test]
    async fn a_pane_may_ask_for_its_own_idle_limit_within_bounds() {
        let state = state();
        let token = issue(&state, PaneOrigin::Core);
        let with_idle = |seconds: &str| {
            let mut dial = Dial::good(&token);
            dial.protocols
                .push(format!("{IDLE_SUBPROTOCOL_PREFIX}{seconds}"));
            inspect(&state, &dial.request())
        };
        let idle = |seconds: &str| with_idle(seconds).unwrap().upstream_idle;
        assert_eq!(idle("30"), Some(Duration::from_secs(30)));
        assert_eq!(idle("1"), Some(Duration::from_secs(MIN_IDLE_SECONDS)));
        assert_eq!(idle("999999"), Some(Duration::from_secs(MAX_IDLE_SECONDS)));
        for bad in ["", "-1", "soon", "1.5"] {
            assert_eq!(
                with_idle(bad).err(),
                Some(StatusCode::BAD_REQUEST),
                "{bad:?}"
            );
        }
        let plain = inspect(&state, &Dial::good(&token).request()).unwrap();
        assert_eq!(plain.upstream_idle, None);
    }
}
