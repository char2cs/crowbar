#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum MemberKind {
    #[default]
    Chat,
    Terminal,
    NewTab,
}
