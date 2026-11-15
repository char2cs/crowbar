use crate::palette_entry::PaletteEntry;

/// An entry that matched the query, with where the query hit its title.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RankedEntry {
    /// The matched entry.
    pub entry: PaletteEntry,
    /// Character positions in the title to highlight.
    pub positions: Vec<usize>,
}
