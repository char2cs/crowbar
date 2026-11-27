#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Highlighting {
    Syntax,
    #[default]
    Semantic,
}
