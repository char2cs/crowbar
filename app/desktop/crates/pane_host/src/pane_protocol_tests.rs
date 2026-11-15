use std::sync::Mutex;
use std::sync::mpsc;

use protocol::{DaemonResponse, ExtensionId, PaneOrigin, RelayEndpoint, RelayScope};
use webview::{
    Bounds, ParentWindow, Visibility, WebViewConfig, WebViewHost, WebViewId, WebViewKind,
};

use super::*;
use crate::bundle_asset::BundleAsset;
use crate::bundle_source::BundleSource;
use crate::extension_bundles::ExtensionBundles;

struct Recording {
    seen: Mutex<Vec<DaemonRequest>>,
    answer: Mutex<Option<protocol::Result<DaemonResponse>>>,
}

impl DaemonTransport for Recording {
    fn request(&self, request: DaemonRequest, respond: protocol::Responder) {
        self.seen.lock().unwrap().push(request);
        let answer = self.answer.lock().unwrap().take();
        respond(answer.unwrap_or(Err(TransportError::Timeout)));
    }

    fn relay_endpoint(&self, _scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        Err(TransportError::Timeout)
    }
}

struct Files;

impl BundleSource for Files {
    fn read(&self, path: &str) -> Option<BundleAsset> {
        (path == "chat/index.html").then(|| BundleAsset {
            mime: "text/html",
            bytes: b"<p>".to_vec(),
        })
    }
}

fn view_ids(count: usize) -> Vec<WebViewId> {
    let host = WebViewHost::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
    (0..count)
        .map(|_| {
            let config = WebViewConfig {
                url: String::new(),
                bounds: Bounds {
                    x: 0.0,
                    y: 0.0,
                    width: 1.0,
                    height: 1.0,
                },
                kind: WebViewKind::Core,
                visibility: Visibility::Hidden,
                initialization_script: None,
                on_ipc: Arc::new(|_| {}),
            };
            host.create_in(ParentWindow::X11(1), config).unwrap().id()
        })
        .collect()
}

struct Fixture {
    protocol: PaneProtocol,
    transport: Arc<Recording>,
    core: WebViewId,
    ext: WebViewId,
}

fn fixture() -> Fixture {
    let transport = Arc::new(Recording {
        seen: Mutex::new(Vec::new()),
        answer: Mutex::new(None),
    });
    let origins = OriginTable::default();
    let ids = view_ids(2);
    origins.set(
        ids[0],
        PaneOrigin::Core,
    );
    origins.set(
        ids[1],
        PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
    );
    let protocol = PaneProtocol::new(
        "crowbar",
        Arc::clone(&transport) as Arc<dyn DaemonTransport>,
        PaneBundles {
            core: Arc::new(Files),
            extensions: ExtensionBundles::default(),
        },
        origins,
    );
    Fixture {
        protocol,
        transport,
        core: ids[0],
        ext: ids[1],
    }
}

fn ask(f: &Fixture, view: WebViewId, uri: &str) -> ProtocolResponse {
    let (sender, receiver) = mpsc::channel();
    let request = ProtocolRequest {
        web_view: view,
        uri: uri.into(),
        method: "GET".into(),
        headers: Vec::new(),
        body: Vec::new(),
    };
    f.protocol.handle(
        request,
        Box::new(move |response| sender.send(response).unwrap()),
    );
    receiver.recv().unwrap()
}

#[test]
fn core_bundle_files_are_served_with_their_media_type() {
    let f = fixture();
    let response = ask(&f, f.core, "crowbar://local/panes/core/chat/index.html");
    assert_eq!(response.status, 200, "status");
    assert!(
        response
            .headers
            .iter()
            .any(|(k, v)| k == "content-type" && v == "text/html"),
        "mime"
    );
    assert_eq!(
        ask(&f, f.core, "crowbar://local/panes/core/nope.js").status,
        404,
        "missing file"
    );
}

#[test]
fn daemon_requests_carry_the_origin_of_the_web_view() {
    let f = fixture();
    *f.transport.answer.lock().unwrap() = Some(Ok(DaemonResponse::new(200, Vec::new(), b"ok".to_vec())));
    let response = ask(&f, f.core, "crowbar://local/v0/health?x=1");
    assert_eq!(
        (response.status, response.body),
        (200, b"ok".to_vec()),
        "passed through"
    );
    let seen = f.transport.seen.lock().unwrap();
    assert_eq!(seen[0].target().as_str(), "/v0/health?x=1", "path");
    assert_eq!(seen[0].origin(), &PaneOrigin::Core, "origin");
}

#[test]
fn extension_webviews_are_confined_to_their_own_routes() {
    let f = fixture();
    assert_eq!(
        ask(&f, f.ext, "crowbar://local/v0/threads").status,
        403,
        "general api"
    );
    assert_eq!(
        ask(&f, f.ext, "crowbar://local/panes/core/chat/index.html").status,
        403,
        "core bundle"
    );
    assert!(
        f.transport.seen.lock().unwrap().is_empty(),
        "nothing reached the daemon"
    );
    *f.transport.answer.lock().unwrap() = Some(Ok(DaemonResponse::new(204, Vec::new(), Vec::new())));
    assert_eq!(
        ask(&f, f.ext, "crowbar://local/v0/ext/git/review.diff").status,
        204,
        "own route"
    );
    let seen = f.transport.seen.lock().unwrap();
    assert_eq!(
        seen[0].origin().extension(),
        ExtensionId::new("git").ok().as_ref(),
        "extension id forwarded"
    );
}

#[test]
fn unknown_views_and_bad_urls_are_refused() {
    let f = fixture();
    let stranger = view_ids(3)[2];
    assert_eq!(
        ask(&f, stranger, "crowbar://local/v0/x").status,
        403,
        "unknown view"
    );
    assert_eq!(
        ask(&f, f.core, "http://local/v0/x").status,
        400,
        "foreign scheme"
    );
}

#[test]
fn transport_errors_map_to_http_statuses() {
    let f = fixture();
    let cases = [
        (TransportError::UnknownHost(HostId::new("x").unwrap()), 404),
        (TransportError::Denied("no"), 403),
        (TransportError::Timeout, 504),
        (TransportError::HostUnavailable("down".into()), 502),
    ];
    for (error, status) in cases {
        *f.transport.answer.lock().unwrap() = Some(Err(error));
        assert_eq!(ask(&f, f.core, "crowbar://local/v0/x").status, status);
    }
}
