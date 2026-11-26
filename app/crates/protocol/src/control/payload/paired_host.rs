use serde::{Deserialize, Serialize};

use crate::control::payload::DisplayText;
use crate::identity::HostId;

/// A host the shell paired with this computer, as a pane page reaches it: by its id under the
/// `crowbar://` scheme, showing its name and address.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PairedHost {
    /// The transport's id for the host.
    pub id: HostId,
    /// The name the user knows it by.
    pub name: DisplayText,
    /// `host:port`, as it was paired.
    pub endpoint: DisplayText,
}
