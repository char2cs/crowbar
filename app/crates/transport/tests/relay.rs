#[path = "support/real_infra/echo_daemon.rs"]
mod echo_daemon;
#[path = "support/memory_listener.rs"]
mod memory_listener;
#[path = "support/relay_rig.rs"]
mod relay_rig;
#[path = "support/ws_client.rs"]
mod ws_client;

use std::future::Future;
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use protocol::{HostId, PaneId, PaneOrigin};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use transport::{RelayConfig, StreamAccess, TOKEN_SUBPROTOCOL_PREFIX};

use crate::echo_daemon::text;
use crate::memory_listener::MemoryDialer;
use crate::relay_rig::{CORE_ORIGIN, RelayRig};
use crate::ws_client::{Client, Dial, status_of};

type TestResult = Result<(), Box<dyn std::error::Error>>;

const STREAM: &str = "/v0/terminals/t1/stream";
const BUDGET: Duration = Duration::from_secs(30);

fn rig() -> Result<RelayRig, Box<dyn std::error::Error>> {
    RelayRig::start(RelayConfig::default())
}

fn good(
    rig: &RelayRig,
    token: &str,
) -> Dial {
    Dial::good(rig.dialer.clone(), CORE_ORIGIN, token, STREAM)
}

async fn refused_with(dial: Dial) -> Result<u16, Box<dyn std::error::Error>> {
    let error = dial
        .connect()
        .await
        .err()
        .ok_or("the relay admitted a dial it should refuse")?;
    Ok(status_of(&error).ok_or("the refusal is not an HTTP status")?)
}

async fn next_close(ws: &mut Client) -> Option<CloseCode> {
    let closes = ws.filter_map(|item| {
        std::future::ready(match item {
            Ok(Message::Close(frame)) => Some(frame.map(|f| f.code)),
            _ => None,
        })
    });
    std::pin::pin!(closes).next().await.flatten()
}

async fn ended_without_data(ws: &mut Client) -> bool {
    let verdicts = ws.filter_map(|item| {
        std::future::ready(match item {
            Ok(Message::Close(_)) => Some(true),
            Ok(Message::Binary(_)) => Some(false),
            _ => None,
        })
    });
    std::pin::pin!(verdicts).next().await.unwrap_or(true)
}

async fn echoes(ws: &mut Client) -> bool {
    ws.send(text("probe")).await.is_ok() && matches!(ws.next().await, Some(Ok(Message::Text(_))))
}

async fn echoing_client(
    rig: &RelayRig,
    token: &str,
) -> Option<Client> {
    let mut ws = good(rig, token).connect().await.ok()?;
    echoes(&mut ws).await.then_some(ws)
}

async fn reconnect(
    rig: &RelayRig,
    token: &str,
) -> Option<Client> {
    let mut client = echoing_client(rig, token).await;
    while client.is_none() {
        tokio::task::yield_now().await;
        client = echoing_client(rig, token).await;
    }
    client
}

async fn refusal(
    rig: &RelayRig,
    token: &str,
) -> Option<u16> {
    status_of(&good(rig, token).connect().await.err()?)
}

async fn refused_once_revoked(
    rig: &RelayRig,
    token: &str,
) -> Option<u16> {
    let mut status = refusal(rig, token).await;
    while status.is_none() {
        tokio::task::yield_now().await;
        status = refusal(rig, token).await;
    }
    status
}

async fn drained(ws: &mut Client) {
    while ws.next().await.is_some() {}
}

async fn within<T>(future: impl Future<Output = T>) -> Result<T, tokio::time::error::Elapsed> {
    tokio::time::timeout(BUDGET, future).await
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn text_and_binary_frames_round_trip_and_keep_their_kind() -> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.send(text("hello")).await.unwrap();
    ws.send(Message::Binary(vec![0, 1, 2, 255].into()))
        .await
        .unwrap();
    assert_eq!(within(ws.next()).await?.unwrap().unwrap(), text("hello"));
    assert_eq!(
        within(ws.next()).await?.unwrap().unwrap(),
        Message::Binary(vec![0, 1, 2, 255].into())
    );
    assert_eq!(rig.daemon.dialled(), [STREAM]);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn large_binary_frames_arrive_byte_identical() -> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    let frame: Vec<u8> = (0..8 * 1024 * 1024)
        .map(|i: usize| u8::try_from(i % 251).unwrap_or(0))
        .collect();
    for _ in 0..4 {
        ws.send(Message::Binary(frame.clone().into()))
            .await
            .unwrap();
        let echoed = within(ws.next()).await?.unwrap().unwrap();
        assert!(
            echoed == Message::Binary(frame.clone().into()),
            "payload changed in transit or came back as another kind"
        );
    }
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn a_message_over_the_cap_ends_the_stream_instead_of_being_buffered() -> TestResult {
    let config = RelayConfig {
        max_message_bytes: 1024,
        ..RelayConfig::default()
    };
    let rig = RelayRig::start(config)?;
    let token = rig.core_token("p1")?;
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.send(Message::Binary(vec![7u8; 4096].into()))
        .await
        .unwrap();
    let ended = within(ended_without_data(&mut ws)).await?;
    assert!(ended, "the oversized frame was forwarded");
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn a_close_from_the_pane_is_echoed_with_its_code() -> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.close(Some(tokio_tungstenite::tungstenite::protocol::CloseFrame {
        code: CloseCode::Normal,
        reason: "done".into(),
    }))
    .await
    .unwrap();
    let closing = within(next_close(&mut ws)).await?;
    assert_eq!(closing, Some(CloseCode::Normal));
    Ok(())
}

type Spoil = fn(&mut Dial);

const CASES: [(&str, Spoil, u16); 14] = [
    (
        "unknown token",
        |dial| {
            dial.protocols.truncate(1);
            dial.protocols
                .push(format!("{TOKEN_SUBPROTOCOL_PREFIX}not-a-token"));
        },
        401,
    ),
    ("missing token", |dial| dial.protocols.truncate(1), 401),
    (
        "missing relay subprotocol",
        |dial| {
            dial.protocols.remove(0);
        },
        400,
    ),
    ("no origin", |dial| dial.origins = vec![], 403),
    (
        "null origin",
        |dial| dial.origins = vec!["null".to_owned()],
        403,
    ),
    (
        "foreign origin",
        |dial| dial.origins = vec!["https://evil.example".to_owned()],
        403,
    ),
    (
        "origin with a suffix",
        |dial| dial.origins = vec![format!("{CORE_ORIGIN}.evil.example")],
        403,
    ),
    (
        "origin with a slash",
        |dial| dial.origins = vec![format!("{CORE_ORIGIN}/")],
        403,
    ),
    (
        "two origins",
        |dial| dial.origins = vec![CORE_ORIGIN.to_owned(), CORE_ORIGIN.to_owned()],
        403,
    ),
    (
        "upper-cased origin",
        |dial| dial.origins = vec![CORE_ORIGIN.to_uppercase()],
        403,
    ),
    (
        "rebound host name",
        |dial| dial.host = Some(format!("evil.example:{}", dial.port)),
        403,
    ),
    (
        "host without a port",
        |dial| dial.host = Some("127.0.0.1".to_owned()),
        403,
    ),
    (
        "localhost instead of the loopback address",
        |dial| dial.host = Some(format!("localhost:{}", dial.port)),
        403,
    ),
    (
        "another port",
        |dial| dial.host = Some(format!("127.0.0.1:{}", dial.port.wrapping_add(1))),
        403,
    ),
];

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn a_dial_that_breaks_one_rule_is_refused_with_that_rules_status() -> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    for (name, spoil, status) in CASES {
        let mut dial = good(&rig, &token);
        spoil(&mut dial);
        assert_eq!(refused_with(dial).await?, status, "{name}");
    }
    assert_eq!(
        rig.daemon.dialled().len(),
        0,
        "the daemon was never dialled"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn an_extension_token_reaches_only_its_own_extension_routes() -> TestResult {
    let rig = rig()?;
    let token = rig.extension_token("x1", "acme")?;
    let origin = "crowbar-ext://acme";
    let dial = |path: &str, origin: &str| Dial::good(rig.dialer.clone(), origin, &token, path);
    assert!(dial("/v0/ext/acme/events", origin).connect().await.is_ok());
    let cases = [
        "/v0/terminals/t1/stream",
        "/v0/ext/other/events",
        "/v0/ext/acme/../terminals/t1",
        "/v0/ext/acme/%2e%2e/x",
        "/v0/ext//acme/x",
        "/v0/ext/acmeevil/x",
    ];
    for path in cases {
        assert_eq!(refused_with(dial(path, origin)).await?, 403, "{path}");
    }
    assert_eq!(
        refused_with(dial("/v0/ext/acme/events", CORE_ORIGIN)).await?,
        403,
        "a core origin cannot use an extension token"
    );
    assert_eq!(rig.daemon.dialled(), ["/v0/ext/acme/events"]);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn a_core_token_cannot_be_used_from_an_extension_origin() -> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    let dial = Dial::good(
        rig.dialer.clone(),
        "crowbar-ext://acme",
        &token,
        "/v0/ext/acme/events",
    );
    assert_eq!(refused_with(dial).await?, 403);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn issuing_again_replaces_the_old_token() -> TestResult {
    let rig = rig()?;
    let old = rig.core_token("p1")?;
    let fresh = rig.core_token("p1")?;
    assert_eq!(refused_with(good(&rig, &old)).await?, 401);
    assert!(good(&rig, &fresh).connect().await.is_ok());
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn revoking_a_pane_closes_its_open_streams_with_a_policy_close_and_kills_its_token()
-> TestResult {
    let rig = rig()?;
    let token = rig.core_token("p1")?;
    let other = rig.core_token("p2")?;
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    let mut bystander = good(&rig, &other).connect().await.expect("admitted");
    ws.send(text("ping")).await.unwrap();
    within(ws.next()).await?.unwrap().unwrap();
    rig.relay.revoke_pane(&PaneId::new("p1").unwrap());
    let closed = within(next_close(&mut ws)).await?;
    assert_eq!(closed, Some(CloseCode::Policy));
    assert_eq!(refused_with(good(&rig, &token)).await?, 401);
    bystander.send(text("still here")).await.unwrap();
    assert_eq!(
        within(bystander.next()).await?.unwrap().unwrap(),
        text("still here")
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn the_connection_caps_hold_and_free_up_when_a_stream_closes() -> TestResult {
    let config = RelayConfig {
        max_per_pane: 2,
        ..RelayConfig::default()
    };
    let rig = RelayRig::start(config)?;
    let token = rig.core_token("p1")?;
    let mut first = good(&rig, &token).connect().await.expect("first");
    let _second = good(&rig, &token).connect().await.expect("second");
    assert_eq!(
        refused_with(good(&rig, &token)).await?,
        503,
        "a third stream of one pane is turned away before the upgrade"
    );
    first.close(None).await.unwrap();
    within(drained(&mut first)).await?;
    let reopened = within(reconnect(&rig, &token))
        .await?
        .ok_or("the pane never reconnected")?;
    drop(reopened);
    Ok(())
}

#[tokio::test(start_paused = true)]
async fn an_expired_token_is_refused() -> TestResult {
    let config = RelayConfig {
        token_ttl: Duration::from_secs(60),
        ..RelayConfig::default()
    };
    let rig = RelayRig::start(config)?;
    let token = rig.core_token("p1")?;
    tokio::time::advance(Duration::from_secs(61)).await;
    assert_eq!(refused_with(good(&rig, &token)).await?, 401);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn removing_a_host_revokes_its_tokens() -> TestResult {
    let rig = rig()?;
    let id = HostId::new("host-1").unwrap();
    let scope = protocol::RelayScope::new(id.clone(), PaneId::new("p1").unwrap(), PaneOrigin::Core);
    let token = rig.relay.issue(&scope).unwrap().token().to_owned();
    rig.transport.remove_host(&id).await.unwrap();
    assert_eq!(within(refused_once_revoked(&rig, &token)).await?, Some(401));
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn the_relay_listens_on_loopback_only_and_names_its_port_in_the_endpoint() -> TestResult {
    let rig = rig()?;
    assert!(rig.relay.addr().is_ipv4() && rig.relay.addr().ip().is_loopback());
    let scope = protocol::RelayScope::new(
        HostId::new("local").unwrap(),
        PaneId::new("p1").unwrap(),
        PaneOrigin::Core,
    );
    let endpoint = rig.relay.issue(&scope).unwrap();
    assert_eq!(
        endpoint.url(),
        format!("ws://127.0.0.1:{}", rig.relay.addr().port())
    );
    assert_eq!(endpoint.token().len(), 43);
    Ok(())
}

async fn until_refused(dialer: &MemoryDialer) {
    while dialer.connect().await.is_ok() {
        tokio::task::yield_now().await;
    }
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn shutting_down_stops_accepting_connections() -> TestResult {
    let rig = rig()?;
    rig.relay.shutdown();
    within(until_refused(&rig.dialer)).await?;
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: relays to a fake daemon listening on a real unix socket"]
async fn a_handshake_beyond_the_limit_is_answered_503_not_left_hanging() -> TestResult {
    let config = RelayConfig {
        max_handshakes: 1,
        handshake_timeout: BUDGET,
        ..RelayConfig::default()
    };
    let rig = RelayRig::start(config)?;
    let silent = rig.dialer.connect().await.unwrap();
    let token = rig.core_token("p1")?;
    assert_eq!(refused_with(good(&rig, &token)).await?, 503);
    drop(silent);
    Ok(())
}
