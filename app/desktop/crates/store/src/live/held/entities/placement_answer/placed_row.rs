use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct PlacedRow {
    pub(crate) id: String,
    pub(crate) parent_id: String,
    pub(crate) order: i64,
}
