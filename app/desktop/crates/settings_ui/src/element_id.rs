//! Element ids built from a prefix and a name.

use gpui::{ElementId, SharedString};

pub(crate) fn element_id(
    prefix: &str,
    name: &str,
) -> ElementId {
    ElementId::from(SharedString::from(format!("{prefix}-{name}")))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ids_join_prefix_and_name_and_differ_by_either() {
        assert_eq!(element_id("row", "a"), element_id("row", "a"));
        assert_ne!(element_id("row", "a"), element_id("row", "b"));
        assert_ne!(element_id("row", "a"), element_id("reset", "a"));
    }
}
