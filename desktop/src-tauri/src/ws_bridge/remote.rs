//! `ws_open` for a paired remote host: the local bridge's upgrade and lifetime,
//! over the pinned TLS dial `crowbar://<hostId>` requests use.

use std::sync::Arc;
use std::time::Duration;

use tauri::http::{header::AUTHORIZATION, HeaderValue};
use tokio::task::JoinHandle;

use super::{attach, upgrade, upgrade_request, FrameSink, WsBridgeManager};
use crate::api_proxy::remote::{bearer, dial, stored_token, DialError, DIAL_TIMEOUT};
use crate::host_tokens::TokenStore;
use crate::hosts::{HostRegistry, RemoteHost};

/// A registered host and the credential its daemon expects.
pub(super) struct PairedHost {
    host: RemoteHost,
    authorization: HeaderValue,
}

/// Read afresh on every open and before anything is dialed, so a host with no
/// credential never sees an upgrade and an unregistered one is unreachable at once.
pub(super) async fn paired_host(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
) -> Result<PairedHost, String> {
    let host = registry
        .get(host_id)
        .ok_or_else(|| format!("unknown remote host {host_id}"))?;
    let token = stored_token(tokens, host_id)
        .await
        .map_err(|e| format!("token lookup failed: {e}"))?
        .ok_or("no stored credential for this host — pair it again")?;
    Ok(PairedHost {
        host,
        authorization: bearer(&token)?,
    })
}

/// [`super::open_bridge`] for a paired host. Failing to reach it, or finding it
/// without its pinned identity, rejects the open as `<marker>: <reason>`, where the
/// marker is the `x-crowbar-proxy` value an HTTP request failing that way carries.
pub(super) async fn open_remote_bridge<S: FrameSink>(
    paired: PairedHost,
    conn_id: String,
    path: String,
    on_message: S,
    manager: &WsBridgeManager,
    window_label: String,
    read_idle_timeout: Option<Duration>,
) -> Result<JoinHandle<()>, String> {
    let mut request = upgrade_request(&path)?;
    request
        .headers_mut()
        .insert(AUTHORIZATION, paired.authorization);
    let tls = dial(&paired.host).await.map_err(rejected)?;
    // The upgrade finishes the handshake, so it gets the dial's bound too.
    let ws = tokio::time::timeout(DIAL_TIMEOUT, upgrade(tls, request))
        .await
        .map_err(|_| {
            rejected(DialError::Unreachable(format!(
                "{} did not answer the upgrade within {DIAL_TIMEOUT:?}",
                paired.host.endpoint
            )))
        })??;
    Ok(attach(
        ws,
        conn_id,
        on_message,
        manager,
        window_label,
        read_idle_timeout,
    ))
}

fn rejected(e: DialError) -> String {
    log::error!("ws_open: {e}");
    format!("{}: {e}", e.marker())
}

#[cfg(test)]
mod tests {
    use std::net::SocketAddr;
    use std::time::Duration;

    use futures_util::SinkExt;
    use rustls::pki_types::{CertificateDer, PrivateKeyDer};
    use tokio::net::TcpListener;
    use tokio::sync::{mpsc, oneshot};
    use tokio::task::JoinHandle;
    use tokio_tungstenite::tungstenite::handshake::server::{
        Callback, ErrorResponse, Request, Response,
    };
    use tokio_tungstenite::tungstenite::Message;

    use super::{open_remote_bridge, paired_host};
    use crate::api_proxy::test_support::{
        header, identity, refused_endpoint, registry, serve_once_tls, tls_acceptor, unrelated_key,
        Keychain, HOST_ID,
    };
    use crate::api_proxy::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};
    use crate::hosts::HostRegistry;
    use crate::tls_pin::fingerprint_of;
    use crate::ws_bridge::{Frame, FrameSink, WsBridgeManager};

    const GREETING: &str = "hello-remote";

    struct Frames(mpsc::UnboundedSender<Frame>);

    impl FrameSink for Frames {
        fn send(&self, frame: Frame) {
            let _ = self.0.send(frame);
        }
    }

    fn frames() -> (Frames, mpsc::UnboundedReceiver<Frame>) {
        let (tx, rx) = mpsc::unbounded_channel();
        (Frames(tx), rx)
    }

    /// The upgrade's path and Authorization header, or why the TLS handshake failed.
    type Seen = Result<(String, Option<String>), String>;

    struct Report(oneshot::Sender<Seen>);

    impl Callback for Report {
        fn on_request(self, req: &Request, resp: Response) -> Result<Response, ErrorResponse> {
            let authorization = req
                .headers()
                .get("authorization")
                .map(|v| v.to_str().unwrap().to_string());
            let _ = self.0.send(Ok((req.uri().to_string(), authorization)));
            Ok(resp)
        }
    }

    /// A remote daemon: terminates TLS as `tls_acceptor` does, upgrades one stream,
    /// reports what it saw, sends `GREETING`, then holds the stream open.
    async fn serve_ws_once(
        cert: CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
    ) -> (SocketAddr, oneshot::Receiver<Seen>) {
        let listener = TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind test remote daemon");
        let addr = listener.local_addr().unwrap();
        let acceptor = tls_acceptor(rustls::DEFAULT_VERSIONS, cert, key);
        let (report, reported) = oneshot::channel();
        tokio::spawn(async move {
            let (tcp, _) = listener.accept().await.expect("accept");
            let tls = match acceptor.accept(tcp).await {
                Ok(tls) => tls,
                Err(e) => {
                    let _ = report.send(Err(e.to_string()));
                    return;
                }
            };
            let mut ws = tokio_tungstenite::accept_hdr_async(tls, Report(report))
                .await
                .expect("upgrade");
            ws.send(Message::Text(GREETING.into()))
                .await
                .expect("send greeting");
            std::future::pending::<()>().await;
        });
        (addr, reported)
    }

    async fn open_as_paired(
        registry: &HostRegistry,
        manager: &WsBridgeManager,
        sink: Frames,
    ) -> Result<JoinHandle<()>, String> {
        let keychain = Keychain::holding(&[(HOST_ID, "sometoken")]);
        let paired = paired_host(registry, &keychain, HOST_ID).await?;
        open_remote_bridge(
            paired,
            "c1".into(),
            "/v0/x".into(),
            sink,
            manager,
            "main".into(),
            None,
        )
        .await
    }

    fn marked(marker: &str, err: &str) -> bool {
        err.starts_with(&format!("{marker}: "))
    }

    #[tokio::test]
    async fn a_stream_reaches_the_pinned_host_with_its_stored_bearer_token() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_ws_once(cert, key).await;
        let registry = registry(addr.to_string(), fingerprint);
        let manager = WsBridgeManager::new();
        let (sink, mut received) = frames();

        let reader = open_as_paired(&registry, &manager, sink)
            .await
            .expect("the pinned host is bridged");

        let (path, authorization) = reported
            .await
            .expect("server reported")
            .expect("handshake with the pinned host");
        assert_eq!(path, "/v0/x");
        assert_eq!(authorization.as_deref(), Some("Bearer sometoken"));
        assert_eq!(received.recv().await, Some(Frame::Text(GREETING.into())));

        manager.connections.lock().unwrap().remove("c1");
        reader
            .await
            .expect("retiring a remote connection must end its reader");
    }

    #[tokio::test]
    async fn a_host_presenting_another_certificate_is_an_identity_change_before_the_upgrade() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let (addr, reported) = serve_ws_once(impostor_cert, impostor_key).await;
        let registry = registry(addr.to_string(), fingerprint_of(&pinned));
        let manager = WsBridgeManager::new();

        let err = open_as_paired(&registry, &manager, frames().0)
            .await
            .expect_err("an impostor must not be bridged");

        assert!(marked(HOST_IDENTITY_CHANGED, &err), "{err}");
        assert!(err.contains("identity changed"), "{err}");
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the impostor completed a handshake and saw the upgrade: {served:?}"
        );
        assert!(manager.connections.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn a_host_replaying_the_pinned_certificate_without_its_key_is_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (addr, reported) = serve_ws_once(pinned.clone(), unrelated_key()).await;
        let registry = registry(addr.to_string(), fingerprint_of(&pinned));
        let manager = WsBridgeManager::new();

        let err = open_as_paired(&registry, &manager, frames().0)
            .await
            .expect_err("a replayed certificate must not be bridged");

        assert!(marked(HOST_IDENTITY_CHANGED, &err), "{err}");
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the replay completed a handshake and saw the upgrade: {served:?}"
        );
        assert!(manager.connections.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn an_unreachable_host_is_told_apart_from_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(refused_endpoint(), "SHA256:unused".into());

        let err = open_as_paired(&registry, &WsBridgeManager::new(), frames().0)
            .await
            .expect_err("nothing listens there");

        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_tls_handshake_is_unreachable_at_the_dial_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        // Listening, so the kernel completes the TCP handshake, but never speaks TLS.
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let registry = registry(silent.local_addr().unwrap().to_string(), "SHA256:x".into());
        let manager = WsBridgeManager::new();

        let opened = tokio::time::timeout(
            Duration::from_secs(60),
            open_as_paired(&registry, &manager, frames().0),
        );
        let err = opened
            .await
            .expect("must fail at the dial deadline, not hang")
            .expect_err("a silent host is not bridged");

        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
        assert!(err.contains("did not answer within"), "{err}");
    }

    #[tokio::test]
    async fn a_host_that_never_answers_the_upgrade_is_unreachable_at_the_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_once_tls(cert, key, None).await;
        let mut opening = tokio::spawn(async move {
            let registry = registry(addr.to_string(), fingerprint);
            let manager = WsBridgeManager::new();
            let opened = open_as_paired(&registry, &manager, frames().0).await;
            let registered = !manager.connections.lock().unwrap().is_empty();
            (opened, registered)
        });

        // Frozen only once the upgrade is in, so the TLS handshake runs on real time.
        let upgrade = tokio::select! {
            seen = reported => seen.unwrap().expect("handshake with the pinned host"),
            ended = &mut opening => panic!("ended before any upgrade: {:?}", ended.unwrap().0),
        };
        assert_eq!(header(&upgrade, "authorization"), Some("Bearer sometoken"));
        tokio::time::pause();
        let (opened, registered) = tokio::time::timeout(Duration::from_secs(600), opening)
            .await
            .expect("must fail at the upgrade deadline, not hang")
            .unwrap();

        let err = opened.expect_err("a host that never upgrades is not bridged");
        assert!(marked(HOST_UNREACHABLE, &err), "{err}");
        assert!(err.contains("did not answer the upgrade"), "{err}");
        assert!(!registered, "a stream that never opened was registered");
    }

    #[tokio::test]
    async fn a_host_without_a_usable_credential_is_refused_before_dialing() {
        let _serialised = crate::test_support::fd_tests().await;
        // A dial would reach the refused endpoint and fail as host-unreachable instead.
        let registry = registry(refused_endpoint(), "SHA256:unused".into());
        let cases = [
            (Keychain::holding(&[]), "no stored credential"),
            (
                Keychain::holding(&[("another-host", "sometoken")]),
                "no stored credential",
            ),
            (Keychain::holding(&[(HOST_ID, "")]), "no stored credential"),
            (
                Keychain::holding(&[(HOST_ID, " \t")]),
                "no stored credential",
            ),
            (
                Keychain::holding(&[(HOST_ID, "some\ntoken")]),
                "not a valid header value",
            ),
            (Keychain::broken("keychain is locked"), "keychain is locked"),
        ];
        for (case, (keychain, reason)) in cases.into_iter().enumerate() {
            let Err(err) = paired_host(&registry, &keychain, HOST_ID).await else {
                panic!("case {case}: a host without a usable credential was paired");
            };
            assert!(err.contains(reason), "case {case}: {err}");
        }
    }

    #[tokio::test]
    async fn an_unregistered_host_is_not_bridged_even_while_reachable() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, _reported) = serve_ws_once(cert, key).await;
        let registry = registry(addr.to_string(), fingerprint);
        registry.remove(HOST_ID);

        let err = open_as_paired(&registry, &WsBridgeManager::new(), frames().0)
            .await
            .expect_err("an unregistered host must not be bridged");

        assert!(err.contains("unknown remote host"), "{err}");
    }
}
