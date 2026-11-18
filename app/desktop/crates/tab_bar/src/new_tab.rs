use layout::TabId;

/// The id and title of a tab a drop creates; the owner supplies an id nothing uses yet.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct NewTab {
    /// The new tab's id.
    pub id: TabId,
    /// Its title.
    pub title: String,
}
