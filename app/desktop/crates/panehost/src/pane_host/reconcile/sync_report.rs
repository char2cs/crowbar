use crate::error::Error;
use crate::slot::HostEvent;

#[derive(Debug, Default)]
pub struct SyncReport {
    pub events: Vec<HostEvent>,
    pub failure: Option<Error>,
}
