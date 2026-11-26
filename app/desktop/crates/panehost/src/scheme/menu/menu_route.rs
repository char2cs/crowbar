//! A page's ask for a native menu: only the shell's own core pages may make it. WebKit names no
//! Origin to the scheme, so the ask must be JSON: a sandboxed frame could only send it after a
//! preflight, which the scheme never grants.

use protocol::PaneId;
use webview::{MenuEntry, ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::scheme::menu::menu_ask::MenuAsk;
use crate::scheme::menu::menu_body::MenuBody;
use crate::scheme::menu::page_entry::PageEntry;
use crate::scheme::menu::pane_menus::PaneMenus;
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
    let entries = entries.into_iter().map(into_menu).collect();
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

fn into_menu(entry: PageEntry) -> MenuEntry {
    match entry {
        PageEntry::Item { label, enabled } => MenuEntry::Item {
            label,
            enabled,
            checked: false,
        },
        PageEntry::Submenu {
            label,
            enabled,
            entries,
        } => MenuEntry::Submenu {
            label,
            enabled,
            entries: entries.into_iter().map(into_menu).collect(),
        },
        PageEntry::Separator => MenuEntry::Separator,
    }
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use super::*;
    use crate::test_support::{Served, request, served};

    const BODY: &str = r#"{"x":12,"y":34,"entries":[{"kind":"item","label":"Delete","enabled":true},{"kind":"separator"},{"kind":"submenu","label":"Align","enabled":true,"entries":[{"kind":"item","label":"Left","enabled":false}]}]}"#;
    const JSON: &str = "application/json";

    fn ask_from(
        f: &Served,
        (view, origin, kind): (webview::Id, Option<&str>, &str),
    ) -> mpsc::Receiver<ProtocolResponse> {
        let origin = origin.map(|origin| ("Origin", origin));
        let headers: Vec<_> = origin.into_iter().chain([("Content-Type", kind)]).collect();
        let asked = request(view, ("POST", "crowbar://local/__crowbar/menu"), &headers);
        f.ask(ProtocolRequest {
            body: BODY.as_bytes().to_vec(),
            ..asked
        })
    }

    fn path_of(answered: &mpsc::Receiver<ProtocolResponse>) -> serde_json::Value {
        let response = answered.recv().unwrap();
        assert_eq!(response.status, 200, "answered");
        serde_json::from_slice(&response.body).unwrap()
    }

    fn item(label: &str) -> MenuEntry {
        let label = label.into();
        MenuEntry::Item {
            label,
            enabled: true,
            checked: false,
        }
    }

    #[test]
    fn a_core_page_asks_for_a_menu_and_hears_the_chosen_entry_or_that_none_was() {
        let f = served();
        let answered = ask_from(&f, (f.core, None, JSON));
        let ask = f.menus.asks().try_recv().unwrap();
        let left = MenuEntry::Item {
            label: "Left".into(),
            enabled: false,
            checked: false,
        };
        let align = MenuEntry::Submenu {
            label: "Align".into(),
            enabled: true,
            entries: vec![left],
        };
        let entries = vec![item("Delete"), MenuEntry::Separator, align];
        assert_eq!(
            (ask.pane.as_str(), ask.at, &ask.entries),
            ("p1", (12.0, 34.0), &entries)
        );
        ask.answer(Some(vec![2, 0]));
        assert_eq!(path_of(&answered), serde_json::json!({ "path": [2, 0] }));
        let dropped = ask_from(&f, (f.core, None, JSON));
        drop(f.menus.asks().try_recv());
        f.menus.asks().close();
        let unheard = ask_from(&f, (f.core, None, JSON));
        for answered in [dropped, unheard] {
            assert_eq!(
                path_of(&answered),
                serde_json::json!({ "path": null }),
                "none"
            );
        }
    }

    #[test]
    fn only_the_shells_own_core_pages_may_ask() {
        let f = served();
        let refused = [
            ("an extension pane", f.ext, None, JSON),
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
