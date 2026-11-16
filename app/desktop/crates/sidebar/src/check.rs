//! Whether a toggle entry of a context menu is switched on.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Check {
    On,
    Off,
}
