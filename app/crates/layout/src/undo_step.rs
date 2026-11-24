use std::collections::{BTreeMap, BTreeSet};

use protocol::PaneId;

use crate::arrangement::Arrangement;
use crate::pane_instance::PaneInstance;
use crate::space_id::SpaceId;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct UndoStep {
    pub(crate) arrangement: Arrangement,
    pub(crate) parked: BTreeMap<PaneId, PaneInstance>,
    pub(crate) spaces: BTreeSet<SpaceId>,
}
