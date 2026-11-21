/// Whether a tab is kept or only previewed, as develop's file tree opens a file with one click.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum TabKeep {
    /// An ordinary tab.
    #[default]
    Kept,
    /// A preview, written in italics until it is kept.
    Preview,
}
