#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum TabAccess {
    #[default]
    Writable,
    ReadOnly,
}
