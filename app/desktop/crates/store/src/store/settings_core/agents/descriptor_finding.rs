use serde::Deserialize;

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default)]
pub(crate) struct DescriptorFinding {
    pub(crate) rule: String,
    pub(crate) severity: String,
    pub(crate) path: String,
    pub(crate) line: u32,
    pub(crate) message: String,
    pub(crate) hint: String,
}
