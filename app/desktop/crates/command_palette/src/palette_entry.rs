/// One thing the palette can run.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PaletteEntry {
    /// The command id handed back when the entry is chosen.
    pub id: String,
    /// The text the query is matched against and the row shows.
    pub title: String,
    /// The shortcut shown on the right, already formatted for the platform.
    pub shortcut: Option<String>,
}
