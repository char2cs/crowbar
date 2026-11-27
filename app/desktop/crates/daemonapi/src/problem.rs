use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub struct Problem {
    pub code: String,
    pub message: String,
}
