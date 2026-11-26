use std::sync::{Arc, Mutex, mpsc};

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, ExtensionId, PaneId, PaneOrigin, RelayEndpoint,
    RelayScope,
};
use webview::{ParentWindow, ProtocolHandler, ProtocolRequest, ProtocolResponse};

use crate::catalog::{BundleAsset, BundleSource, ExtensionBundles, PaneBundles};
use crate::scheme::{OriginTable, PaneMenus, PaneProtocol, PaneTables};
use crate::test_support::blank_config;

/// A daemon that records what reaches it and answers with what the test set.
#[derive(Debug, Default)]
pub(crate) struct Recording {
    pub(crate) seen: Mutex<Vec<DaemonRequest>>,
    pub(crate) answer: Mutex<Option<protocol::Result<DaemonResponse>>>,
}

impl DaemonTransport for Recording {
    fn request(
        &self,
        request: DaemonRequest,
        respond: protocol::Responder,
    ) {
        self.seen.lock().unwrap().push(request);
        let answer = self.answer.lock().unwrap().take();
        respond(answer.unwrap_or(Err(protocol::Error::Timeout)));
    }

    fn relay_endpoint(
        &self,
        scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        let url = format!("ws://127.0.0.1:1/{}", scope.pane());
        Ok(RelayEndpoint::new(url, "token"))
    }
}

/// A bundle holding one file.
#[derive(Debug)]
struct Files {
    only: &'static str,
}

impl BundleSource for Files {
    fn read(
        &self,
        path: &str,
    ) -> Option<BundleAsset> {
        (path == self.only).then(|| BundleAsset {
            mime: "text/html",
            bytes: b"<p>".to_vec(),
        })
    }
}

/// A router with one core view (pane p1) and one view of the git extension (pane p2).
pub(crate) struct Served {
    pub(crate) protocol: PaneProtocol,
    pub(crate) transport: Arc<Recording>,
    pub(crate) menus: PaneMenus,
    pub(crate) core: webview::Id,
    pub(crate) ext: webview::Id,
    pub(crate) stranger: webview::Id,
}

impl Served {
    pub(crate) fn ask(
        &self,
        request: ProtocolRequest,
    ) -> mpsc::Receiver<ProtocolResponse> {
        let (sender, receiver) = mpsc::channel();
        let respond = Box::new(move |response| sender.send(response).unwrap());
        self.protocol.handle(request, respond);
        receiver
    }

    pub(crate) fn get(
        &self,
        view: webview::Id,
        uri: &str,
    ) -> ProtocolResponse {
        self.ask(request(view, ("GET", uri), &[])).recv().unwrap()
    }
}

pub(crate) fn request(
    web_view: webview::Id,
    (method, uri): (&str, &str),
    headers: &[(&str, &str)],
) -> ProtocolRequest {
    ProtocolRequest {
        web_view,
        uri: uri.into(),
        method: method.into(),
        headers: headers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect(),
        body: Vec::new(),
    }
}

pub(crate) fn header_of<'a>(
    response: &'a ProtocolResponse,
    name: &str,
) -> Option<&'a str> {
    response
        .headers
        .iter()
        .find(|(key, _)| key.eq_ignore_ascii_case(name))
        .map(|(_, value)| value.as_str())
}

pub(crate) fn served() -> Served {
    let host = webview::Host::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
    let view = || {
        let config = blank_config(webview::Kind::Core);
        host.create_in(ParentWindow::X11(1), config).unwrap().id()
    };
    let (core, ext, stranger) = (view(), view(), view());
    let origins = OriginTable::default();
    origins.set(core, PaneId::new("p1").unwrap(), PaneOrigin::Core);
    let git = PaneOrigin::Extension(ExtensionId::new("git").unwrap());
    origins.set(ext, PaneId::new("p2").unwrap(), git);
    let transport = Arc::new(Recording::default());
    let menus = PaneMenus::default();
    let only = "chat/index.html";
    let bundles = PaneBundles {
        core: Arc::new(Files { only }),
        extensions: ExtensionBundles::default(),
    };
    let tables = PaneTables {
        origins,
        menus: menus.clone(),
    };
    let daemon = Arc::clone(&transport) as Arc<dyn DaemonTransport>;
    let protocol = PaneProtocol::new("crowbar", daemon, bundles, tables);
    Served {
        protocol,
        transport,
        menus,
        core,
        ext,
        stranger,
    }
}
