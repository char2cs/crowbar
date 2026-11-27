#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub(crate) enum Phase {
    #[default]
    Shown,
    Leaving,
}
