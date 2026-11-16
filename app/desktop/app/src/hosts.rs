#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Hosts {
    pub(crate) has_local: bool,
    pub(crate) paired: usize,
}
