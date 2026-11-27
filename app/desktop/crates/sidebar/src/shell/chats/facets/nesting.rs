#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Nesting {
    #[default]
    Leaf,
    Parent,
}
