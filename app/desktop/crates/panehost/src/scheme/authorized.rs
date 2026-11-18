use protocol::{PaneId, PaneOrigin};
use webview::ProtocolRequest;

use crate::scheme::route::Route;

#[derive(Debug)]
pub(crate) struct Authorized {
    pub(crate) request: ProtocolRequest,
    pub(crate) pane: PaneId,
    pub(crate) origin: PaneOrigin,
    pub(crate) route: Route,
}
