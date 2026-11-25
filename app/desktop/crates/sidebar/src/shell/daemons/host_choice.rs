//! The hosts the header's picker offers.

use crate::shell::daemons::host::Host;
use crate::shell::daemons::host_id::HostId;

/// The hosts the header's picker offers, and the one it shows.
pub trait HostChoice {
    /// Every known host.
    fn hosts(&self) -> Vec<Host>;
    /// The host whose spaces are listed, whether or not it has listed any yet.
    fn shown_host(&self) -> Option<HostId>;
}
