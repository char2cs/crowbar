//! A page's ask for a native menu: only the shell's own core pages may make it. WebKit names no
//! Origin to the scheme, so the ask must be JSON: a sandboxed frame could only send it after a
//! preflight, which the scheme never grants.

use protocol::PaneId;
use webview::{ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::scheme::menu_ask::MenuAsk;
use crate::scheme::menu_body::MenuBody;
use crate::scheme::page_entry::PageEntry;
use crate::scheme::pane_menus::PaneMenus;
use crate::scheme::route::Route;
use crate::scheme::status;

const ASK_METHOD: &str = "POST";
const ASK_TYPE: &str = "application/json";

pub(crate) fn may_ask(
    request: &ProtocolRequest,
    route: &Route,
) -> bool {
    *route != Route::Menu
        || request
            .headers
            .iter()
            .any(|(key, value)| key.eq_ignore_ascii_case("content-type") && value == ASK_TYPE)
}

pub(crate) fn ask(
    menus: &PaneMenus,
    (pane, request): (PaneId, ProtocolRequest),
    respond: ProtocolResponder,
) {
    let body = request
        .method
        .eq_ignore_ascii_case(ASK_METHOD)
        .then(|| serde_json::from_slice::<MenuBody>(&request.body).ok())
        .flatten();
    let Some(MenuBody { entries, x, y }) = body else {
        return respond(answer(status::BAD_REQUEST, &serde_json::json!({})));
    };
    let entries = entries.into_iter().map(PageEntry::into_menu).collect();
    let reply = Box::new(move |path: Option<Vec<usize>>| {
        respond(answer(status::OK, &serde_json::json!({ "path": path })));
    });
    menus.send(MenuAsk::new(pane, (entries, (x, y)), reply));
}

fn answer(
    code: u16,
    body: &serde_json::Value,
) -> ProtocolResponse {
    ProtocolResponse {
        status: code,
        headers: vec![("content-type".into(), "application/json".into())],
        body: body.to_string().into_bytes(),
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, mpsc};

    use protocol::{DaemonTransport, ExtensionId, PaneOrigin};
    use webview::{MenuEntry, ParentWindow, ProtocolHandler};

    use super::*;
    use crate::catalog::{ExtensionBundles, PaneBundles};
    use crate::scheme::{OriginTable, PaneProtocol, PaneTables};
    use crate::test_support::{Revocations, blank_config};

    const URI: &str = "crowbar://local/__crowbar/menu";
    const BODY: &str = r#"{"x":12,"y":34,"entries":[{"kind":"item","label":"Delete","enabled":true},{"kind":"separator"},{"kind":"submenu","label":"Align","enabled":true,"entries":[{"kind":"item","label":"Left","enabled":false}]}]}"#;

    struct Fixture {
        protocol: PaneProtocol,
        menus: PaneMenus,
        core: webview::Id,
        extension: webview::Id,
    }

    fn fixture() -> Fixture {
        let host = webview::Host::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
        let mut ids = (0..2).map(|_| {
            let config = blank_config(webview::Kind::Core);
            host.create_in(ParentWindow::X11(1), config).unwrap().id()
        });
        let (core, extension) = (ids.next().unwrap(), ids.next().unwrap());
        let origins = OriginTable::default();
        origins.set(core, PaneId::new("p1").unwrap(), PaneOrigin::Core);
        let git = PaneOrigin::Extension(ExtensionId::new("git").unwrap());
        origins.set(extension, PaneId::new("p2").unwrap(), git);
        let menus = PaneMenus::default();
        let bundles = PaneBundles {
            core: Arc::new(crate::catalog::DirBundle::new("/nonexistent")),
            extensions: ExtensionBundles::default(),
        };
        let transport = Arc::new(Revocations::default()) as Arc<dyn DaemonTransport>;
        let tables = PaneTables {
            origins,
            menus: menus.clone(),
        };
        let protocol = PaneProtocol::new("crowbar", transport, bundles, tables);
        Fixture {
            protocol,
            menus,
            core,
            extension,
        }
    }

    const JSON: &str = "application/json";

    fn ask_from(
        f: &Fixture,
        (view, origin, kind): (webview::Id, Option<&str>, &str),
    ) -> mpsc::Receiver<ProtocolResponse> {
        let (sender, receiver) = mpsc::channel();
        let request = ProtocolRequest {
            web_view: view,
            uri: URI.into(),
            method: "POST".into(),
            headers: origin
                .map(|origin| ("Origin".to_owned(), origin.to_owned()))
                .into_iter()
                .chain([("Content-Type".to_owned(), kind.to_owned())])
                .collect(),
            body: BODY.as_bytes().to_vec(),
        };
        f.protocol.handle(
            request,
            Box::new(move |response| sender.send(response).unwrap()),
        );
        receiver
    }

    fn body_of(response: &ProtocolResponse) -> serde_json::Value {
        serde_json::from_slice(&response.body).unwrap()
    }

    #[test]
    fn a_core_page_asks_for_a_menu_and_hears_the_chosen_entry() {
        let f = fixture();
        let answered = ask_from(&f, (f.core, None, JSON));
        let heard = f.menus.asks().try_recv();
        assert!(heard.is_ok(), "the shell heard the ask");
        let ask = heard.unwrap();
        assert_eq!(ask.pane, PaneId::new("p1").unwrap());
        assert_eq!(ask.at, (12.0, 34.0));
        let left = MenuEntry::Item {
            label: "Left".into(),
            enabled: false,
            checked: false,
        };
        let entries = vec![
            MenuEntry::Item {
                label: "Delete".into(),
                enabled: true,
                checked: false,
            },
            MenuEntry::Separator,
            MenuEntry::Submenu {
                label: "Align".into(),
                enabled: true,
                entries: vec![left],
            },
        ];
        assert_eq!(ask.entries, entries);
        ask.answer(Some(vec![2, 0]));
        let response = answered.recv().unwrap();
        assert_eq!(response.status, 200);
        assert_eq!(body_of(&response), serde_json::json!({ "path": [2, 0] }));
    }

    #[test]
    fn only_the_shells_own_core_pages_may_ask() {
        let f = fixture();
        let refused = [
            ("an extension pane", f.extension, None, JSON),
            (
                "a frame that names a null origin",
                f.core,
                Some("null"),
                JSON,
            ),
            (
                "a sandboxed frame's preflight-free post",
                f.core,
                None,
                "text/plain",
            ),
        ];
        for (who, view, origin, kind) in refused {
            let answered = ask_from(&f, (view, origin, kind)).try_recv();
            let status = answered.map(|response| response.status);
            assert_eq!(status, Ok(403), "{who} is refused at once");
        }
        assert!(f.menus.asks().is_empty(), "no menu was asked for");
    }
}
