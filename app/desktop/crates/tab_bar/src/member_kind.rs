/// What a strip member is, which picks its glyph.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum MemberKind {
    /// An agent chat.
    #[default]
    Chat,
    /// A terminal session.
    Terminal,
    /// The New Tab page.
    NewTab,
}
