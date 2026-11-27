use async_channel::Sender;
use protocol::PaneId;

use crate::ends::Ends;
use crate::report::Report;

#[derive(Debug, Clone)]
pub struct Connection {
    pub pane: PaneId,
    pub ends: Ends,
    pub reports: Sender<Report>,
}
