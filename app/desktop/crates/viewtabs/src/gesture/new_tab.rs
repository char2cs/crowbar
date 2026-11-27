use layout::TabId;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct NewTab {
    pub id: TabId,
    pub title: String,
}
