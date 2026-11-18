use layout::TabId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct NewTab {
    /// The new tab's id.
    pub id: TabId,
    /// Its title.
    pub title: String,
}
