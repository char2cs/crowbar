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

#[path = "support/http_daemon.rs"]
mod http_daemon;
#[path = "support/real_infra/tls_host.rs"]
mod tls_host;

use std::sync::Arc;

use http::{Method, Request};
use protocol::{HostId, PaneOrigin};
use transport::{
    Endpoint, Error, HostRegistry, MemoryTokenStore, PairRequest, TokenStore, Transport,
    fingerprint_of, pair_remote_host,
};

use crate::http_daemon::Handler;
use crate::tls_host::{TlsHost, identity};

const TOKEN: &str = "tok-1";

fn handler() -> Handler {
    Arc::new(|seen| match (seen.method.as_str(), seen.target.as_str()) {
        ("POST", "/v0/pair") if seen.body.windows(4).any(|w| w == b"good") => (
            200,
            vec![],
            r#"{"data":{"token":"tok-1","clientId":"c1"}}"#.into(),
        ),
        ("POST", "/v0/pair") => (403, vec![], r#"{"error":"code expired"}"#.into()),
        (_, _) if seen.header("authorization") == Some("Bearer tok-1") => {
            (200, vec![], r#"{"ok":true}"#.into())
        },
        _ => (401, vec![], "{}".into()),
    })
}

#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn host() -> TlsHost {
    TlsHost::start(identity(), &[&rustls::version::TLS13], handler()).await
}

fn request<'a>(
    endpoint: &'a str,
    fingerprint: &'a str,
    code: &'a str,
) -> PairRequest<'a> {
    PairRequest {
        host_id: "host-1",
        endpoint,
        fingerprint,
        code,
        name: "test shell",
    }
}

struct Rig {
    registry: Arc<HostRegistry>,
    tokens: Arc<dyn TokenStore>,
    transport: Transport,
}

#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
fn rig() -> Rig {
    let registry = Arc::new(HostRegistry::in_memory());
    let tokens: Arc<dyn TokenStore> = Arc::new(MemoryTokenStore::default());
    let transport = Transport::new(
        Arc::new(std::path::PathBuf::from(
            "/crowbar-test-missing/remote.sock",
        )),
        Arc::clone(&registry),
        Arc::clone(&tokens),
    );
    Rig {
        registry,
        tokens,
        transport,
    }
}

#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn pair(
    rig: &Rig,
    host: &TlsHost,
    fingerprint: &str,
    code: &str,
) -> Result<String, Error> {
    let endpoint = host.endpoint();
    pair_remote_host(
        &rig.registry,
        &rig.tokens,
        &request(&endpoint, fingerprint, code),
    )
    .await
}

fn get(path: &str) -> Request<Vec<u8>> {
    Request::builder()
        .method(Method::GET)
        .uri(path)
        .body(Vec::new())
        .expect("request")
}

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn pairing_with_the_confirmed_fingerprint_registers_the_host_and_requests_carry_the_token() {
    let host = host().await;
    let rig = rig();
    let client = pair(&rig, &host, &fingerprint_of(&host.cert_der), "good")
        .await
        .expect("paired");
    assert_eq!(client, "c1");
    let id = HostId::new("host-1").unwrap();
    assert!(rig.registry.get(&id).is_some());
    assert!(
        rig.tokens
            .load_entry("host-1")
            .unwrap()
            .is_some_and(|entry| entry.contains(TOKEN))
    );
    let answer = rig
        .transport
        .handle(&PaneOrigin::Core, get("crowbar://host-1/v0/projects"))
        .await;
    assert_eq!(answer.status(), 200);
    let sent = host.requests().pop().expect("the host saw a request");
    assert_eq!(sent.header("authorization"), Some("Bearer tok-1"));
}

enum Fingerprint {
    Pinned,
    Other,
}

type Refusal = fn(&Error) -> bool;

/// (fingerprint the shell confirmed, code typed, how the failure reads, requests the host saw)
type Case = (Fingerprint, &'static str, Refusal, usize);

const CASES: [Case; 2] = [
    (
        Fingerprint::Other,
        "good",
        |error| matches!(error, Error::IdentityChanged(_)),
        0,
    ),
    (
        Fingerprint::Pinned,
        "stale",
        |error| error.to_string().contains("code expired"),
        1,
    ),
];

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn a_refused_pairing_is_reported_and_leaves_no_trace() {
    for (fingerprint, code, reads_as, seen) in CASES {
        let host = host().await;
        let rig = rig();
        let confirmed = match fingerprint {
            Fingerprint::Pinned => fingerprint_of(&host.cert_der),
            Fingerprint::Other => fingerprint_of(&identity().cert),
        };
        let error = pair(&rig, &host, &confirmed, code).await.unwrap_err();
        assert!(reads_as(&error), "{code}: {error}");
        assert_eq!(rig.registry.list().len(), 0);
        assert!(rig.tokens.load_entry("host-1").unwrap().is_none());
        assert_eq!(
            host.requests().len(),
            seen,
            "{code}: requests that reached the host"
        );
    }
}

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn a_host_that_changes_its_certificate_after_pairing_never_sees_the_token() {
    let host = host().await;
    let rig = rig();
    pair(&rig, &host, &fingerprint_of(&host.cert_der), "good")
        .await
        .expect("paired");
    let impostor = TlsHost::start(identity(), &[&rustls::version::TLS13], handler()).await;
    let id = HostId::new("host-1").unwrap();
    rig.registry
        .insert(id, Endpoint::parse(&impostor.endpoint()).unwrap())
        .await
        .unwrap();
    let answer = rig
        .transport
        .handle(&PaneOrigin::Core, get("crowbar://host-1/v0/projects"))
        .await;
    assert_eq!(transport::marker_of(&answer), Some("host-identity-changed"));
    assert_eq!(
        impostor.requests().len(),
        0,
        "the bearer token never reached the impostor"
    );
}

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn a_host_that_only_speaks_tls12_is_not_dialled_with_the_token() {
    let old = TlsHost::start(identity(), &[&rustls::version::TLS12], handler()).await;
    let rig = rig();
    let error = pair(&rig, &old, &fingerprint_of(&old.cert_der), "good")
        .await
        .unwrap_err();
    assert!(!matches!(error, Error::Refused { .. }), "{error}");
    assert_eq!(old.requests().len(), 0);
}

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn an_unreachable_host_is_marked_unreachable() {
    let host = host().await;
    let rig = rig();
    pair(&rig, &host, &fingerprint_of(&host.cert_der), "good")
        .await
        .expect("paired");
    let closed = std::net::TcpListener::bind(("127.0.0.1", 0)).unwrap();
    let endpoint = Endpoint::parse(&closed.local_addr().unwrap().to_string()).unwrap();
    drop(closed);
    rig.registry
        .insert(HostId::new("host-1").unwrap(), endpoint)
        .await
        .unwrap();
    let answer = rig
        .transport
        .handle(&PaneOrigin::Core, get("crowbar://host-1/v0/projects"))
        .await;
    assert_eq!(transport::marker_of(&answer), Some("host-unreachable"));
}

#[derive(Debug)]
struct FullKeychain {
    complaint: &'static str,
}

impl TokenStore for FullKeychain {
    fn load_entry(
        &self,
        _host_id: &str,
    ) -> Result<Option<String>, Error> {
        Ok(None)
    }

    fn set(
        &self,
        _host_id: &str,
        _entry: &str,
    ) -> Result<(), Error> {
        Err(Error::Store(self.complaint.into()))
    }

    fn delete(
        &self,
        _host_id: &str,
    ) -> Result<(), Error> {
        Ok(())
    }
}

#[tokio::test]
#[ignore = "real-infra: pairs and dials a fake host over a loopback TLS socket"]
async fn a_token_the_keychain_cannot_keep_is_revoked_on_the_host() {
    let host = host().await;
    let registry = Arc::new(HostRegistry::in_memory());
    let tokens: Arc<dyn TokenStore> = Arc::new(FullKeychain {
        complaint: "the keychain is full",
    });
    let endpoint = host.endpoint();
    let fingerprint = fingerprint_of(&host.cert_der);
    let pairing = request(&endpoint, &fingerprint, "good");
    let error = pair_remote_host(&registry, &tokens, &pairing)
        .await
        .unwrap_err();
    assert!(matches!(error, Error::NotRecorded { .. }), "{error}");
    assert_eq!(registry.list().len(), 0);
    let revoke = host
        .requests()
        .into_iter()
        .find(|seen| seen.method == "DELETE")
        .expect("revoked on the host");
    assert_eq!(revoke.target, "/v0/clients/c1");
    assert_eq!(revoke.header("authorization"), Some("Bearer tok-1"));
}
