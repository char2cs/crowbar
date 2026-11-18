/// What draws a pane type.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum PaneKind {
    /// An entry page in a web view the host pools.
    #[default]
    Web,
    /// A GPUI view the shell paints itself; the host assigns no web view and sends no
    /// lifecycle frames.
    Native,
}
