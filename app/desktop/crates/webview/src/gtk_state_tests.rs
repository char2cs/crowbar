use std::sync::Arc;

use super::*;
use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
use crate::protocol_request::ProtocolRequest;

struct Silent;

impl ProtocolHandler for Silent {
    fn handle(&self, _request: ProtocolRequest, _respond: ProtocolResponder) {}
}

#[test]
#[ignore = "needs display"]
fn a_parent_that_is_not_an_x11_window_is_refused() {
    gtk::init().unwrap();
    let mut state = GtkState::new(HostConfig {
        scheme: "crowbar".into(),
        protocol: Arc::new(Silent),
        data_dir: None,
    });
    let config = WebViewConfig {
        url: "about:blank".into(),
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
    let id = WebViewId(1);
    let refused = state.create(id, ParentWindow::AppKit(1), &config);
    assert!(
        matches!(refused, Err(Error::UnsupportedParent(_))),
        "refused"
    );
}
