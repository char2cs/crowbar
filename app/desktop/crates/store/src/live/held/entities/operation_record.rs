mod entity_ref;
mod item;

pub(crate) use item::Item;

use daemonapi::Problem;
use serde::Deserialize;

use crate::live::held::entities::operation_record::entity_ref::EntityRef;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct OperationRecord {
    pub(crate) id: String,
    pub(crate) kind: String,
    pub(crate) subject: EntityRef,
    pub(crate) project_id: String,
    pub(crate) label: String,
    pub(crate) state: String,
    pub(crate) items: Vec<Item>,
    pub(crate) error: Option<Problem>,
    pub(crate) dismissed: bool,
    pub(crate) created_at: String,
}
