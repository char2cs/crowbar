/// One row of a [`ContextMenu`](crate::ContextMenu).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MenuRow {
    /// A row that can be picked.
    Item {
        /// Its text, shown as written.
        label: String,
        /// Greyed and inert when false.
        enabled: bool,
        /// Drawn with the system's check mark.
        checked: bool,
    },
    /// A row that cascades into the rows under it.
    Submenu {
        /// Its text, shown as written.
        label: String,
        /// Greyed and closed when false.
        enabled: bool,
        /// The rows under it.
        rows: Vec<MenuRow>,
    },
    /// A dividing line.
    Separator,
}
