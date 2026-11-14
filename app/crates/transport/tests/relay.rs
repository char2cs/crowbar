//! Public-API tests of `transport` against real sockets and fake daemons.
#![allow(
    clippy::expect_used,
    clippy::unwrap_used,
    clippy::indexing_slicing,
    clippy::string_slice,
    clippy::result_large_err,
    clippy::excessive_nesting,
    reason = "test code: a failing fake or assertion should fail the test loudly, and async blocks nest by construction"
)]

#[path = "support/echo_daemon.rs"]
mod echo_daemon;
#[path = "support/relay_rig.rs"]
mod relay_rig;
#[path = "support/ws_client.rs"]
mod ws_client;

use std::future::Future;
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use protocol::{HostId, PaneId, PaneKind};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use transport::{RelayConfig, StreamAccess};

use crate::echo_daemon::text;
use crate::relay_rig::{CORE_ORIGIN, RelayRig};
use crate::ws_client::{Client, Dial, status_of};

const STREAM: &str = "/v0/terminals/t1/stream";
const BUDGET: Duration = Duration::from_secs(30);

async fn rig(tag: &str) -> RelayRig {
    RelayRig::start(tag, RelayConfig::default()).await
}

fn good(rig: &RelayRig, token: &str) -> Dial {
    Dial::good(rig.relay.addr().port(), CORE_ORIGIN, token, STREAM)
}

async fn refused_with(dial: Dial) -> u16 {
    let error = dial.connect().await.expect_err("the relay refuses the dial");
    status_of(&error).expect("an HTTP refusal")
}

async fn next_close(ws: &mut Client) -> Option<CloseCode> {
    while let Some(Ok(message)) = ws.next().await {
        if let Message::Close(frame) = message {
            return frame.map(|f| f.code);
        }
    }
    None
}

async fn ended_without_data(ws: &mut Client) -> bool {
    while let Some(Ok(message)) = ws.next().await {
        match message {
            Message::Close(_) => return true,
            Message::Binary(_) => return false,
            _ => {}
        }
    }
    true
}

async fn echoes(ws: &mut Client) -> bool {
    ws.send(text("probe")).await.is_ok() && matches!(ws.next().await, Some(Ok(Message::Text(_))))
}

async fn reconnect(rig: &RelayRig, token: &str) -> Client {
    loop {
        if let Ok(mut ws) = good(rig, token).connect().await
            && echoes(&mut ws).await
        {
            return ws;
        }
        tokio::task::yield_now().await;
    }
}

async fn refused_once_revoked(rig: &RelayRig, token: &str) -> u16 {
    loop {
        match good(rig, token).connect().await {
            Err(error) => return status_of(&error).expect("an HTTP refusal"),
            Ok(_) => tokio::task::yield_now().await,
        }
    }
}

async fn within<T>(future: impl Future<Output = T>) -> T {
    tokio::time::timeout(BUDGET, future)
        .await
        .expect("finished in time")
}

#[tokio::test]
async fn text_and_binary_frames_round_trip_and_keep_their_kind() {
    let rig = rig("kinds").await;
    let token = rig.core_token("p1");
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.send(text("hello")).await.unwrap();
    ws.send(Message::Binary(vec![0, 1, 2, 255].into()))
        .await
        .unwrap();
    assert_eq!(within(ws.next()).await.unwrap().unwrap(), text("hello"));
    assert_eq!(
        within(ws.next()).await.unwrap().unwrap(),
        Message::Binary(vec![0, 1, 2, 255].into())
    );
    assert_eq!(rig.daemon.dialled(), [STREAM]);
}

#[tokio::test]
async fn large_binary_frames_arrive_byte_identical() {
    let rig = rig("large").await;
    let token = rig.core_token("p1");
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    let frame: Vec<u8> = (0..8 * 1024 * 1024)
        .map(|i: usize| u8::try_from(i % 251).unwrap_or(0))
        .collect();
    for _ in 0..4 {
        ws.send(Message::Binary(frame.clone().into()))
            .await
            .unwrap();
        let Message::Binary(echoed) = within(ws.next()).await.unwrap().unwrap() else {
            panic!("a binary frame came back as another kind");
        };
        assert_eq!(echoed.len(), frame.len());
        assert!(
            echoed.as_ref() == frame.as_slice(),
            "payload changed in transit"
        );
    }
}

#[tokio::test]
async fn a_message_over_the_cap_ends_the_stream_instead_of_being_buffered() {
    let config = RelayConfig {
        max_message_bytes: 1024,
        ..RelayConfig::default()
    };
    let rig = RelayRig::start("cap", config).await;
    let token = rig.core_token("p1");
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.send(Message::Binary(vec![7u8; 4096].into()))
        .await
        .unwrap();
    let ended = within(ended_without_data(&mut ws)).await;
    assert!(ended, "the oversized frame was forwarded");
}

#[tokio::test]
async fn a_close_from_the_pane_is_echoed_with_its_code() {
    let rig = rig("close").await;
    let token = rig.core_token("p1");
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    ws.close(Some(tokio_tungstenite::tungstenite::protocol::CloseFrame {
        code: CloseCode::Normal,
        reason: "done".into(),
    }))
    .await
    .unwrap();
    let closing = within(next_close(&mut ws)).await;
    assert_eq!(closing, Some(CloseCode::Normal));
}

#[tokio::test]
async fn an_unknown_missing_or_unrelated_token_is_refused_401() {
    let rig = rig("token").await;
    assert_eq!(refused_with(good(&rig, "not-a-token")).await, 401);
    let mut none = good(&rig, "x");
    none.protocols.truncate(1);
    assert_eq!(refused_with(none).await, 401);
    assert_eq!(
        rig.daemon.dialled().len(),
         0,
        "the daemon was never dialled"
    );
}

#[tokio::test]
async fn a_missing_relay_subprotocol_is_a_bad_request() {
    let rig = rig("proto").await;
    let token = rig.core_token("p1");
    let mut dial = good(&rig, &token);
    dial.protocols.remove(0);
    assert_eq!(refused_with(dial).await, 400);
}

#[tokio::test]
async fn origin_must_be_present_single_and_exact() {
    let rig = rig("origin").await;
    let token = rig.core_token("p1");
    for origins in [
        vec![],
        vec!["null".to_owned()],
        vec!["https://evil.example".to_owned()],
        vec![format!("{CORE_ORIGIN}.evil.example")],
        vec![format!("{CORE_ORIGIN}/")],
        vec![CORE_ORIGIN.to_owned(), CORE_ORIGIN.to_owned()],
        vec![CORE_ORIGIN.to_uppercase()],
    ] {
        let mut dial = good(&rig, &token);
        dial.origins = origins.clone();
        assert_eq!(refused_with(dial).await, 403, "{origins:?}");
    }
    assert_eq!(rig.daemon.dialled().len(), 0);
}

#[tokio::test]
async fn a_rebound_host_header_is_refused_before_anything_else() {
    let rig = rig("host").await;
    let token = rig.core_token("p1");
    let port = rig.relay.addr().port();
    for host in [
        format!("evil.example:{port}"),
        "127.0.0.1".to_owned(),
        format!("localhost:{port}"),
        format!("127.0.0.1:{}", port.wrapping_add(1)),
    ] {
        let mut dial = good(&rig, &token);
        dial.host = Some(host.clone());
        assert_eq!(refused_with(dial).await, 403, "{host}");
    }
}

#[tokio::test]
async fn an_extension_token_reaches_only_its_own_extension_routes() {
    let rig = rig("ext").await;
    let token = rig.token("x1", PaneKind::Extension, Some("acme"));
    let port = rig.relay.addr().port();
    let origin = "crowbar-ext://acme";
    let dial = |path: &str, origin: &str| Dial::good(port, origin, &token, path);
    assert!(dial("/v0/ext/acme/events", origin).connect().await.is_ok());
    for path in [
        "/v0/terminals/t1/stream",
        "/v0/ext/other/events",
        "/v0/ext/acme/../terminals/t1",
        "/v0/ext/acme/%2e%2e/x",
        "/v0/ext//acme/x",
        "/v0/ext/acmeevil/x",
    ] {
        assert_eq!(refused_with(dial(path, origin)).await, 403, "{path}");
    }
    assert_eq!(
        refused_with(dial("/v0/ext/acme/events", CORE_ORIGIN)).await,
        403,
        "a core origin cannot use an extension token"
    );
    assert_eq!(rig.daemon.dialled(), ["/v0/ext/acme/events"]);
}

#[tokio::test]
async fn a_core_token_cannot_be_used_from_an_extension_origin() {
    let rig = rig("coreonly").await;
    let token = rig.core_token("p1");
    let dial = Dial::good(
        rig.relay.addr().port(),
        "crowbar-ext://acme",
        &token,
        "/v0/ext/acme/events",
    );
    assert_eq!(refused_with(dial).await, 403);
}

#[tokio::test]
async fn issuing_again_replaces_the_old_token() {
    let rig = rig("replace").await;
    let old = rig.core_token("p1");
    let fresh = rig.core_token("p1");
    assert_eq!(refused_with(good(&rig, &old)).await, 401);
    assert!(good(&rig, &fresh).connect().await.is_ok());
}

#[tokio::test]
async fn revoking_a_pane_closes_its_open_streams_with_a_policy_close_and_kills_its_token() {
    let rig = rig("revoke").await;
    let token = rig.core_token("p1");
    let other = rig.core_token("p2");
    let mut ws = good(&rig, &token).connect().await.expect("admitted");
    let mut bystander = good(&rig, &other).connect().await.expect("admitted");
    ws.send(text("ping")).await.unwrap();
    within(ws.next()).await.unwrap().unwrap();
    rig.relay.revoke_pane(&PaneId::new("p1").unwrap());
    let closed = within(next_close(&mut ws)).await;
    assert_eq!(closed, Some(CloseCode::Policy));
    assert_eq!(refused_with(good(&rig, &token)).await, 401);
    bystander.send(text("still here")).await.unwrap();
    assert_eq!(
        within(bystander.next()).await.unwrap().unwrap(),
        text("still here")
    );
}

#[tokio::test]
async fn the_connection_caps_hold_and_free_up_when_a_stream_closes() {
    let config = RelayConfig {
        max_per_pane: 2,
        ..RelayConfig::default()
    };
    let rig = RelayRig::start("caps", config).await;
    let token = rig.core_token("p1");
    let mut first = good(&rig, &token).connect().await.expect("first");
    let _second = good(&rig, &token).connect().await.expect("second");
    let mut third = good(&rig, &token).connect().await.expect("handshake completes");
    let closed = within(next_close(&mut third)).await;
    assert_eq!(closed, Some(CloseCode::Again), "a third stream of one pane is turned away");
    first.close(None).await.unwrap();
    within(async { while first.next().await.is_some() {} }).await;
    let reopened = within(reconnect(&rig, &token)).await;
    drop(reopened);
}

#[tokio::test(start_paused = true)]
async fn an_expired_token_is_refused() {
    let config = RelayConfig {
        token_ttl: Duration::from_secs(60),
        ..RelayConfig::default()
    };
    let rig = RelayRig::start("ttl", config).await;
    let token = rig.core_token("p1");
    tokio::time::advance(Duration::from_secs(61)).await;
    assert_eq!(refused_with(good(&rig, &token)).await, 401);
}

#[tokio::test]
async fn removing_a_host_revokes_its_tokens() {
    let rig = rig("hostgone").await;
    let id = HostId::new("host-1").unwrap();
    let scope = protocol::RelayScope {
        host: id.clone(),
        pane: PaneId::new("p1").unwrap(),
        kind: PaneKind::Core,
        extension: None,
    };
    let token = rig.relay.issue(&scope).unwrap().token;
    rig.transport.remove_host(&id).unwrap();
    assert_eq!(within(refused_once_revoked(&rig, &token)).await, 401);
}
