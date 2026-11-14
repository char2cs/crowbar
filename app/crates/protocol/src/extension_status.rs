use serde::{Deserialize, Serialize};

use crate::extension_id::ExtensionId;
use crate::status_kind::StatusKind;

/// Payload of an `extension.status` frame.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExtensionStatus {
    /// The extension the status is about.
    pub extension_id: ExtensionId,
    /// The new status.
    pub status: StatusKind,
    /// Why the extension is not running.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
}
