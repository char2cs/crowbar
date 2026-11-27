use crate::shell::daemons::host::Host;
use crate::shell::daemons::host_id::HostId;

pub trait HostChoice {
    fn hosts(&self) -> Vec<Host>;
    fn shown_host(&self) -> Option<HostId>;
}
