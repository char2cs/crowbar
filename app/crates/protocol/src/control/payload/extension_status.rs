pub(super) mod status_kind;

use serde::{Deserialize, Serialize};

use crate::control::payload::extension_status::status_kind::StatusKind;
use crate::control::payload::text::display_text::DisplayText;
use crate::identity::ExtensionId;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExtensionStatus {
    extension_id: ExtensionId,
    status: StatusKind,
    #[serde(skip_serializing_if = "Option::is_none")]
    reason: Option<DisplayText>,
}

impl ExtensionStatus {
    #[must_use]
    pub fn new(
        extension_id: ExtensionId,
        status: StatusKind,
        reason: Option<DisplayText>,
    ) -> Self {
        Self {
            extension_id,
            status,
            reason,
        }
    }

    #[must_use]
    pub fn extension_id(&self) -> &ExtensionId {
        &self.extension_id
    }

    #[must_use]
    pub fn status(&self) -> StatusKind {
        self.status
    }

    #[must_use]
    pub fn reason(&self) -> Option<&DisplayText> {
        self.reason.as_ref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn matches_the_host_protocol_shape() {
        let status = ExtensionStatus::new(
            ExtensionId::new("git").unwrap(),
            StatusKind::Unavailable,
            Some(DisplayText::new("crashed").unwrap()),
        );
        let text = serde_json::to_string(&status).unwrap();
        assert_eq!(
            text, r#"{"extensionId":"git","status":"unavailable","reason":"crashed"}"#,
            "shape"
        );
        assert_eq!(
            serde_json::from_str::<ExtensionStatus>(&text).unwrap(),
            status,
            "round trip"
        );
    }

    #[test]
    fn a_running_extension_has_no_reason() {
        let status =
            ExtensionStatus::new(ExtensionId::new("git").unwrap(), StatusKind::Running, None);
        assert_eq!(
            serde_json::to_string(&status).unwrap(),
            r#"{"extensionId":"git","status":"running"}"#,
            "reason omitted"
        );
    }
}
