//! Looks up an embedded icon by its Phosphor file stem.

use ui::IconName;

pub(crate) fn icon_named(stem: &str) -> Option<IconName> {
    IconName::ALL
        .iter()
        .copied()
        .find(|icon| icon.stem() == stem)
}

#[cfg(test)]
mod tests {
    use super::*;
    use settings::default_schema;

    #[test]
    fn finds_known_stems_only() {
        assert_eq!(icon_named("paint-brush"), Some(IconName::PaintBrush));
        assert_eq!(icon_named("not-an-icon"), None);
    }

    #[test]
    fn every_icon_the_catalog_names_is_embedded() {
        let schema = default_schema().unwrap();
        for page in schema.pages() {
            assert!(icon_named(&page.icon).is_some(), "page {}", page.id());
        }
    }
}
