//! Where keyboard input is going when it reaches the modal.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum KeyMode {
    Capturing,
    Rail,
    Page,
}
