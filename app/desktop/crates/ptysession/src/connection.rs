//! What a terminal view hands a session: its pane, its ends and where to report.

use async_channel::Sender;
use protocol::PaneId;

use crate::ends::Ends;
use crate::report::Report;

/// A view's side of a session.
#[derive(Debug, Clone)]
pub struct Connection {
    /// The pane the view shows in; the relay scope is cut to it.
    pub pane: PaneId,
    /// Where frames go and where the view's messages come from.
    pub ends: Ends,
    /// Where the session tells how binding and streaming went.
    pub reports: Sender<Report>,
}
