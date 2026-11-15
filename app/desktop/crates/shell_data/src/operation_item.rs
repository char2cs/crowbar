//! One entity an operation creates or changes.

use serde::Deserialize;

use crate::entity_ref::EntityRef;
use crate::problem::Problem;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct OperationItem {
    #[serde(rename = "ref")]
    pub(crate) target: EntityRef,
    pub(crate) label: String,
    pub(crate) state: String,
    pub(crate) error: Option<Problem>,
    pub(crate) parent_id: Option<String>,
}
