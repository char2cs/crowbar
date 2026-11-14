//! Looks up an embedded icon by its Phosphor file stem.

use ui::IconName;

pub(crate) fn icon_named(stem: &str) -> Option<IconName> {
    IconName::ALL
        .iter()
        .copied()
        .find(|icon| icon.stem() == stem)
}

#[cfg(test)]
#[path = "icon_stem_tests.rs"]
mod tests;
