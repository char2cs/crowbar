use protocol::PaneTypeId;

use crate::catalog::multiplicity::Multiplicity;
use crate::catalog::pane_kind::PaneKind;
use crate::catalog::pane_lifecycle::PaneLifecycle;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PaneType {
    pub id: PaneTypeId,
    pub title: String,
    pub entry: String,
    pub kind: PaneKind,
    pub lifecycle: PaneLifecycle,
    pub multiplicity: Multiplicity,
}
