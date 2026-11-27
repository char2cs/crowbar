//! A native menu that remembers each request and answers with one entry.

use gpui::TestAppContext;
use ui::testing::{Popped, answer_menus};
use ui::{MenuPath, NativeEntry};

pub(crate) fn answer_menus_with(
    cx: &mut TestAppContext,
    path: Option<MenuPath>,
) -> Popped {
    answer_menus(cx, move |_| path.clone())
}

pub(crate) fn labels(entries: &[NativeEntry]) -> Vec<String> {
    entries
        .iter()
        .map(|entry| match entry {
            NativeEntry::Item { label, .. } | NativeEntry::Submenu { label, .. } => {
                label.to_string()
            },
            NativeEntry::Separator => "-".to_owned(),
        })
        .collect()
}
