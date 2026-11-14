use std::collections::BTreeMap;

use protocol::PaneId;

use crate::arrangement::Arrangement;
use crate::pane_instance::PaneInstance;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct UndoStep {
    pub(crate) arrangement: Arrangement,
    pub(crate) parked: BTreeMap<PaneId, PaneInstance>,
}
