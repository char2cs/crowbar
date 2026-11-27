use gpui::FocusHandle;

#[derive(Debug)]
pub(crate) struct State {
    pub(crate) open: bool,
    pub(crate) focus: FocusHandle,
}
