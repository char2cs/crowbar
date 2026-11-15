use model::Layout;

#[derive(Debug, Default)]
pub(crate) struct PendingLayout {
    pub(crate) latest: Option<Layout>,
    pub(crate) closed: bool,
}
