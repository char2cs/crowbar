//! What the shell keeps of a paired host that the transport does not: its name, the id its
//! pairing has on the host, and the fingerprint it was paired against.

use serde::{Deserialize, Serialize};

/// A paired host's name, client id and confirmed fingerprint.
#[derive(Debug, Clone, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct Note {
    /// The name the user gave the host.
    pub name: String,
    /// The id of this computer's pairing on the host, which a revoke names.
    pub client_id: String,
    /// The certificate fingerprint the user confirmed.
    pub fingerprint: String,
}
