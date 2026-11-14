//! Element ids built from a prefix and a name.

use gpui::{ElementId, SharedString};

pub(crate) fn element_id(prefix: &str, name: &str) -> ElementId {
    ElementId::from(SharedString::from(format!("{prefix}-{name}")))
}

#[cfg(test)]
#[path = "element_id_tests.rs"]
mod tests;
