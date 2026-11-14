use serde::{Deserialize, Serialize};

/// Who authored a pane: the app itself or an extension.
///
/// Relay tokens and origins are scoped by this value.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum PaneKind {
    /// Shipped with the app, may use the full daemon API.
    Core,
    /// Provided by an extension, restricted to its own routes.
    Extension,
}
