#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Caret {
    pub line: u32,
    pub column: u32,
}
