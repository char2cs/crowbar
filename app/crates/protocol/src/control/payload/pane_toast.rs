pub(super) mod toast_tone;

use serde::{Deserialize, Serialize};

use crate::control::payload::DisplayText;
use crate::control::payload::ToastTone;

/// A toast a pane asks the shell to show, as its `shell.toast` frame carries it.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneToast {
    /// The pane's own id for the toast; the same key rewrites the toast in place.
    pub key: DisplayText,
    /// What the toast reports.
    pub tone: ToastTone,
    /// The headline.
    pub title: DisplayText,
    /// The detail under the headline, if any.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<DisplayText>,
    /// Whether it stays until closed instead of timing out.
    #[serde(default)]
    pub pinned: bool,
}
