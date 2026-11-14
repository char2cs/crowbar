use protocol::HostId;
use serde::{Deserialize, Serialize};

use crate::endpoint::Endpoint;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct HostEntry {
    pub(crate) id: HostId,
    pub(crate) endpoint: Endpoint,
}
