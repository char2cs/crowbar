use tauri::http::header::AUTHORIZATION;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::task::JoinHandle;
use tokio_tungstenite::tungstenite::handshake::client::Request;
use tokio_tungstenite::WebSocketStream;

use super::attach::attach;
use super::frame_sink::FrameSink;
use super::open_request::OpenRequest;
use super::paired_host::PairedHost;
use super::upgrade::{upgrade, upgrade_request};
use super::ws_bridge_manager::WsBridgeManager;
use crate::api_proxy::remote::{dial, DialError, RemoteHost, DIAL_TIMEOUT};
use crate::host_error::HostError;

fn rejected(error: DialError) -> HostError {
    log::error!("ws_open: {error}");
    HostError::Rejected {
        marker: error.marker(),
        source: Box::new(error),
    }
}

async fn upgraded_within_deadline<T>(
    stream: T,
    request: Request,
    host: &RemoteHost,
) -> Result<WebSocketStream<T>, HostError>
where
    T: AsyncRead + AsyncWrite + Unpin,
{
    tokio::time::timeout(DIAL_TIMEOUT, upgrade(stream, request))
        .await
        .map_err(|elapsed| {
            rejected(DialError::Unreachable(format!(
                "{} did not answer the upgrade within {DIAL_TIMEOUT:?} ({elapsed})",
                host.endpoint
            )))
        })?
}

pub(super) async fn open_remote_bridge<S: FrameSink>(
    paired: PairedHost,
    on_message: S,
    manager: &WsBridgeManager,
    request: OpenRequest,
) -> Result<JoinHandle<()>, HostError> {
    let mut handshake = upgrade_request(&request.path)?;
    handshake
        .headers_mut()
        .insert(AUTHORIZATION, paired.authorization);
    let tls = dial(&paired.host).await.map_err(rejected)?;
    let ws = upgraded_within_deadline(tls, handshake, &paired.host).await?;
    let request = OpenRequest {
        host_id: Some(paired.host_id),
        ..request
    };
    Ok(attach(ws, on_message, manager, request))
}

#[cfg(test)]
mod tests {
    use std::net::SocketAddr;
    use std::time::Duration;

    use futures_util::{SinkExt, StreamExt};
    use rustls::pki_types::{CertificateDer, PrivateKeyDer};
    use tokio::sync::oneshot;
    use tokio_rustls::TlsAcceptor;
    use tokio_tungstenite::tungstenite::handshake::server::{
        Callback, ErrorResponse, Request as ServerRequest, Response,
    };
    use tokio_tungstenite::tungstenite::Message;

    use super::*;
    use crate::api_proxy::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};
    use crate::hosts::endpoint::parse_endpoint;
    use crate::hosts::HostRegistry;
    use crate::test_support::bridge_fakes::{frames, silent_sink, ws_pair, Frames};
    use crate::test_support::proxy_fakes::{
        header, identity, paired, refused_endpoint, registry, serve_once_tls, tls_acceptor,
        unrelated_key, Keychain, HOST_ID,
    };
    use crate::ws_bridge::frame::Frame;
    use crate::ws_bridge::paired_host::paired_host;
    use crate::ws_bridge::ws_bridge_manager::locked;

    const GREETING: &str = "hello-remote";
    const OTHER_HOST: &str = "host-2";

    type Seen = Result<(String, Option<String>), String>;

    struct Report(oneshot::Sender<Seen>);

    impl Callback for Report {
        fn on_request(
            self,
            req: &ServerRequest,
            resp: Response,
        ) -> Result<Response, ErrorResponse> {
            let authorization = req
                .headers()
                .get("authorization")
                .map(|v| v.to_str().unwrap().to_owned());
            drop(self.0.send(Ok((req.uri().to_string(), authorization))));
            Ok(resp)
        }
    }

    struct Target<'a> {
        host_id: &'a str,
        cert: &'a [u8],
        conn_id: &'a str,
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn serve_session(
        listener: tokio::net::TcpListener,
        acceptor: TlsAcceptor,
        report: oneshot::Sender<Seen>,
    ) {
        let (tcp, _) = listener.accept().await.expect("accept");
        let Ok(tls) = acceptor.accept(tcp).await else {
            report.send(Err("tls handshake failed".to_owned())).ok();
            return;
        };
        let mut ws = tokio_tungstenite::accept_hdr_async(tls, Report(report))
            .await
            .expect("upgrade");
        ws.send(Message::Text(GREETING.into()))
            .await
            .expect("greet");
        while let Some(Ok(frame)) = ws.next().await {
            ws.send(frame).await.ok();
        }
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn serve_ws_once(
        cert: CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
    ) -> (SocketAddr, oneshot::Receiver<Seen>) {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind");
        let addr = listener.local_addr().unwrap();
        let acceptor = tls_acceptor(rustls::DEFAULT_VERSIONS, cert, key);
        let (report, reported) = oneshot::channel();
        tokio::spawn(serve_session(listener, acceptor, report));
        (addr, reported)
    }

    fn request(conn_id: &str) -> OpenRequest {
        OpenRequest {
            conn_id: conn_id.to_owned(),
            path: "/v0/x".to_owned(),
            host_id: None,
            window_label: "main".to_owned(),
            read_idle_timeout: None,
        }
    }

    async fn open_to(
        target: Target<'_>,
        registry: &HostRegistry,
        manager: &WsBridgeManager,
        sink: Frames,
    ) -> Result<JoinHandle<()>, HostError> {
        let stored = paired("sometoken", target.cert);
        let keychain = Keychain::holding(&[(target.host_id, &stored)]);
        let pair = paired_host(registry, &keychain, target.host_id).await?;
        open_remote_bridge(pair, sink, manager, request(target.conn_id)).await
    }

    async fn open_as_paired(
        registry: &HostRegistry,
        cert: &[u8],
        manager: &WsBridgeManager,
        sink: Frames,
    ) -> Result<JoinHandle<()>, HostError> {
        let target = Target {
            host_id: HOST_ID,
            cert,
            conn_id: "c1",
        };
        open_to(target, registry, manager, sink).await
    }

    fn marked(
        marker: &str,
        err: &HostError,
    ) -> bool {
        err.to_string().starts_with(&format!("{marker}: "))
    }

    #[tokio::test]
    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn a_stream_reaches_the_pinned_host_with_its_stored_bearer_token() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, reported) = serve_ws_once(cert.clone(), key).await;
        let registry = registry(&addr.to_string());
        let manager = WsBridgeManager::default();
        let (sink, mut received) = frames();
        let reader = open_as_paired(&registry, &cert, &manager, sink)
            .await
            .expect("the pinned host is bridged");
        let (path, authorization) = reported.await.unwrap().expect("handshake");
        assert_eq!(path, "/v0/x");
        assert_eq!(authorization.as_deref(), Some("Bearer sometoken"));
        assert_eq!(received.recv().await, Some(Frame::Text(GREETING.into())));
        tokio::time::pause();
        tokio::time::advance(DIAL_TIMEOUT * 3).await;
        manager
            .sender("c1")
            .expect("still registered")
            .send(Message::Text("still-here".into()))
            .unwrap();
        assert_eq!(
            received.recv().await,
            Some(Frame::Text("still-here".into()))
        );
        assert!(!reader.is_finished());
        manager.retire("c1");
        reader.await.expect("retiring ends its reader");
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn two_remote_hosts(registry: &HostRegistry) -> (Vec<u8>, Vec<u8>) {
        let (cert, key) = identity();
        let (addr, _reported) = serve_ws_once(cert.clone(), key).await;
        registry
            .insert(HOST_ID.into(), parse_endpoint(&addr.to_string()).unwrap())
            .unwrap();
        let (other_cert, other_key) = identity();
        let (addr, _reported) = serve_ws_once(other_cert.clone(), other_key).await;
        registry
            .insert(
                OTHER_HOST.into(),
                parse_endpoint(&addr.to_string()).unwrap(),
            )
            .unwrap();
        (cert.to_vec(), other_cert.to_vec())
    }

    #[tokio::test]
    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn removing_a_host_retires_its_streams_and_no_others() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());
        let (cert, other_cert) = two_remote_hosts(&registry).await;
        let manager = WsBridgeManager::default();
        let removed = Target {
            host_id: HOST_ID,
            cert: &cert,
            conn_id: "removed",
        };
        let other = Target {
            host_id: OTHER_HOST,
            cert: &other_cert,
            conn_id: "other",
        };
        let removed = open_to(removed, &registry, &manager, frames().0)
            .await
            .unwrap();
        let other = open_to(other, &registry, &manager, frames().0)
            .await
            .unwrap();
        let (local_ws, _daemon) = ws_pair().await;
        let local_request = OpenRequest {
            host_id: None,
            ..request("local")
        };
        let local = attach(local_ws, silent_sink(), &manager, local_request);
        manager.close_for_host(HOST_ID);
        let mut left: Vec<String> = locked(&manager.connections).keys().cloned().collect();
        left.sort();
        assert_eq!(left, ["local", "other"]);
        removed.await.expect("a removed host's reader must end");
        manager.close_for_window("main");
        other.await.unwrap();
        local.await.unwrap();
    }

    fn served_identity(
        kind: &str,
        pinned: &CertificateDer<'static>,
    ) -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
        match kind {
            "impostor" => identity(),
            _ => (pinned.clone(), unrelated_key()),
        }
    }

    #[tokio::test]
    async fn a_host_not_proving_the_pinned_identity_is_refused_before_the_upgrade() {
        a_host_not_proving_the_pinned_identity_i_run().await;
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn a_host_not_proving_the_pinned_identity_i_run() {
        const CASES: &[&str] = &["impostor", "replay"];
        for kind in CASES {
            let _serialised = crate::test_support::fd_tests().await;
            let (pinned, _) = identity();
            let (cert, key) = served_identity(kind, &pinned);
            let (addr, reported) = serve_ws_once(cert, key).await;
            let registry = registry(&addr.to_string());
            let manager = WsBridgeManager::default();
            let result = open_as_paired(&registry, &pinned, &manager, frames().0).await;
            let err = result.expect_err(kind);
            assert!(marked(HOST_IDENTITY_CHANGED, &err), "{kind}: {err}");
            assert!(
                reported.await.unwrap().is_err(),
                "{kind}: the upgrade was seen"
            );
            assert!(locked(&manager.connections).is_empty(), "{kind}");
        }
    }

    #[tokio::test]
    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn an_unreachable_host_is_told_apart_from_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());
        let manager = WsBridgeManager::default();
        let result = open_as_paired(&registry, b"unused", &manager, frames().0).await;
        let err = result.expect_err("nothing listens there");
        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
    }

    #[tokio::test(start_paused = true)]
    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn a_host_that_never_completes_the_tls_handshake_is_unreachable_at_the_dial_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let registry = registry(&silent.local_addr().unwrap().to_string());
        let manager = WsBridgeManager::default();
        let opening = open_as_paired(&registry, b"unused", &manager, frames().0);
        let opened = tokio::time::timeout(Duration::from_secs(60), opening).await;
        let err = opened
            .expect("must fail at the deadline")
            .expect_err("silent host");
        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
        assert!(err.to_string().contains("did not answer within"), "{err}");
    }

    #[tokio::test]
    async fn a_host_that_never_answers_the_upgrade_is_unreachable_at_the_deadline() {
        a_host_that_never_answers_the_upgrade_is_run().await;
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn a_host_that_never_answers_the_upgrade_is_run() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, reported) = serve_once_tls(cert.clone(), key, None).await;
        let opening = tokio::spawn(async move {
            let registry = registry(&addr.to_string());
            let manager = WsBridgeManager::default();
            let opened = open_as_paired(&registry, &cert, &manager, frames().0).await;
            let registered = !locked(&manager.connections).is_empty();
            (opened, registered)
        });
        let upgrade = reported
            .await
            .expect("the bridge ended before any upgrade")
            .expect("handshake with the pinned host");
        assert_eq!(header(&upgrade, "authorization"), Some("Bearer sometoken"));
        tokio::time::pause();
        let waited = tokio::time::timeout(Duration::from_secs(600), opening).await;
        let (opened, registered) = waited.expect("must fail at the deadline").unwrap();
        let err = opened.expect_err("a host that never upgrades is not bridged");
        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
        assert!(
            err.to_string().contains("did not answer the upgrade"),
            "{err}"
        );
        assert!(!registered, "a stream that never opened was registered");
    }
}
