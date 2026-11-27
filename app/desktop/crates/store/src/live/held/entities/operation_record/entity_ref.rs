use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct EntityRef {
    pub(crate) kind: String,
    pub(crate) id: String,
}
