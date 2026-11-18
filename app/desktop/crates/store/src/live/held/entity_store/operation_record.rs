//! Work that outlives its request, as the daemon records it.

mod entity_ref;
mod operation_item;

pub(crate) use operation_item::OperationItem;

use daemonapi::Problem;
use serde::Deserialize;

use crate::live::held::entity_store::operation_record::entity_ref::EntityRef;

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
