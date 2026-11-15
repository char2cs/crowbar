//! Work that outlives its request, as the daemon records it.

use serde::Deserialize;

use crate::entity_ref::EntityRef;
use crate::operation_item::OperationItem;
use crate::problem::Problem;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct OperationRecord {
    pub(crate) id: String,
    pub(crate) kind: String,
    pub(crate) subject: EntityRef,
    pub(crate) project_id: String,
    pub(crate) label: String,
    pub(crate) state: String,
    pub(crate) items: Vec<OperationItem>,
    pub(crate) error: Option<Problem>,
    pub(crate) dismissed: bool,
    pub(crate) created_at: String,
}
