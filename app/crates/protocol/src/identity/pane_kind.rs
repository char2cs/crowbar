use serde::{Deserialize, Serialize};

/// Who authored a pane: the app itself or an extension.
///
/// A coarse category of a [`PaneOrigin`](crate::PaneOrigin), for code that only needs to tell
/// core from extension; scopes themselves use `PaneOrigin`, which cannot name no extension.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum PaneKind {
    /// Shipped with the app, may use the full daemon API.
    Core,
    /// Provided by an extension, restricted to its own routes.
    Extension,
}
