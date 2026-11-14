use serde::{Deserialize, Serialize};

use crate::display_text::DisplayText;
use crate::extension_id::ExtensionId;
use crate::status_kind::StatusKind;

/// Payload of an `extension.status` frame.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExtensionStatus {
    extension_id: ExtensionId,
    status: StatusKind,
    #[serde(skip_serializing_if = "Option::is_none")]
    reason: Option<DisplayText>,
}

impl ExtensionStatus {
    /// The status of `extension_id`, with the reason it is not running if any.
    #[must_use]
    pub fn new(extension_id: ExtensionId, status: StatusKind, reason: Option<DisplayText>) -> Self {
        Self {
            extension_id,
            status,
            reason,
        }
    }

    /// The extension the status is about.
    #[must_use]
    pub fn extension_id(&self) -> &ExtensionId {
        &self.extension_id
    }

    /// The new status.
    #[must_use]
    pub fn status(&self) -> StatusKind {
        self.status
    }

    /// Why the extension is not running.
    #[must_use]
    pub fn reason(&self) -> Option<&DisplayText> {
        self.reason.as_ref()
    }
}

#[cfg(test)]
#[path = "extension_status_tests.rs"]
mod tests;
