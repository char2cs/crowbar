#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Reach {
    #[default]
    Launching,
    Reached,
}
