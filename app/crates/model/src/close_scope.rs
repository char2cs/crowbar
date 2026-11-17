/// Which tabs of a strip a bulk close reaches, counted from the tab it was started on.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CloseScope {
    /// Every tab except the one it was started on.
    Others,
    /// The tabs after the one it was started on.
    ToRight,
    /// Every tab, the one it was started on included.
    All,
}
