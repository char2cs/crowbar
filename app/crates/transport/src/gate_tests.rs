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

fn issue(state: &RelayState, origin: PaneOrigin) -> String {
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
