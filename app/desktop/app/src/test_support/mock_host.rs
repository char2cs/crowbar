use std::rc::Rc;
use std::sync::Arc;

use panehost::{OriginTable, PaneHost, PaneHostConfig};
use protocol::HostId;
use webview::{MockPlatform, WebViewHost};

use super::NoDaemon;
use crate::core_panes::register_core_panes;

pub(crate) fn mock_host(mock: &Rc<MockPlatform>) -> PaneHost {
    let config = PaneHostConfig {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
        page_handler: None,
    };
    let webviews = WebViewHost::with_platform(Rc::<MockPlatform>::clone(mock));
    let mut host = PaneHost::new(
        config,
        webviews,
        OriginTable::default(),
        Arc::new(NoDaemon::timing_out()),
    );
    register_core_panes(host.registry_mut(), "chat/index.html").unwrap();
    host
}
