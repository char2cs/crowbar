//! One entry of a native menu, as the desktop draws it.

use gpui::SharedString;

/// An entry of a [`NativeRequest`](crate::NativeRequest): plain text, no icon and no colour, as
/// develop's GTK menus show.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum NativeEntry {
    /// A row that can be chosen.
    Item {
        /// Its text.
        label: SharedString,
        /// Greyed and inert when false.
        enabled: bool,
        /// Drawn with the desktop's check mark.
        checked: bool,
    },
    /// A row that cascades into the entries under it.
    Submenu {
        /// Its text.
        label: SharedString,
        /// Greyed and closed when false.
        enabled: bool,
        /// The entries it opens.
        entries: Vec<NativeEntry>,
    },
    /// A hairline between groups.
    Separator,
}
