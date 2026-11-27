use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct PointerItem {
    pub s: String,
    pub tag: String,
    pub role: String,
    pub name: String,
    pub text: String,
    pub r: Vec<i64>,
    pub h: String,
    pub i: i64,
}
