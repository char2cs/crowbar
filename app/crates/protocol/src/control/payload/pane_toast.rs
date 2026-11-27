pub(super) mod toast_tone;

use serde::{Deserialize, Serialize};

use crate::control::payload::DisplayText;
use crate::control::payload::ToastTone;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneToast {
    pub key: DisplayText,
    pub tone: ToastTone,
    pub title: DisplayText,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<DisplayText>,
    #[serde(default)]
    pub pinned: bool,
}
