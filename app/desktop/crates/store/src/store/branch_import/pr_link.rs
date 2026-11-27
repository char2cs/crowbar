use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct PrLink {
    pub(crate) head: String,
    pub(crate) base: String,
}
