use std::rc::Rc;
use std::sync::Arc;

use panehost::PaneHost;
use protocol::HostId;
use webview::MockPlatform;

use super::NoDaemon;
use crate::panes::kinds::core_panes::register_core_panes;

pub(crate) fn mock_host(mock: &Rc<MockPlatform>) -> PaneHost {
    let config = panehost::Config {
        scheme: "crowbar".into(),
        host: HostId::new("local").unwrap(),
        max_idle_slots: 1,
        page_handler: None,
    };
    let webviews = webview::Host::with_platform(Rc::<MockPlatform>::clone(mock));
    let mut host = PaneHost::new(
        config,
        webviews,
        panehost::PaneTables::default(),
        Arc::new(NoDaemon::timing_out()),
    );
    register_core_panes(host.registry_mut(), "chat/index.html").unwrap();
    host
}
