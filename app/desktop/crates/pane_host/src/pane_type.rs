use protocol::PaneTypeId;

use crate::multiplicity::Multiplicity;
use crate::pane_lifecycle::PaneLifecycle;

/// A registered pane type. Built-in and extension panes are the same thing to the host: an
/// entry page and the rules for its life.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PaneType {
    /// Namespaced id, `core.chat` or `ext.<extension>.<pane>`.
    pub id: PaneTypeId,
    /// Display name, also used by the placeholder.
    pub title: String,
    /// Entry page path inside the owner's bundle, for example `chat/index.html`.
    pub entry: String,
    /// What happens to the web view when the pane is hidden.
    pub lifecycle: PaneLifecycle,
    /// How many instances may exist.
    pub multiplicity: Multiplicity,
}
