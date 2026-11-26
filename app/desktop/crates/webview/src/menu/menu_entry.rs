/// One entry of a native context menu, drawn by the desktop: plain text, no icon.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MenuEntry {
    /// A row that can be chosen.
    Item {
        /// Its text, shown as written (no mnemonic).
        label: String,
        /// Greyed and inert when false.
        enabled: bool,
        /// Drawn with the desktop's check mark.
        checked: bool,
    },
    /// A row that cascades into the entries under it.
    Submenu {
        /// Its text, shown as written (no mnemonic).
        label: String,
        /// Greyed and closed when false.
        enabled: bool,
        /// The entries it opens.
        entries: Vec<MenuEntry>,
    },
    /// A hairline between groups.
    Separator,
}
