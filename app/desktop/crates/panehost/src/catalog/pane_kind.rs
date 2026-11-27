#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum PaneKind {
    #[default]
    Web,
    Native,
}
