use serde::{Deserialize, Serialize};

use crate::control::payload::DisplayText;
use crate::identity::HostId;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PairedHost {
    pub id: HostId,
    pub name: DisplayText,
    pub endpoint: DisplayText,
}
