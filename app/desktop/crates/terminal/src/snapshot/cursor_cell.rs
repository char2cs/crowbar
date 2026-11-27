use crate::snapshot::cursor_shape::CursorShape;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CursorCell {
    pub row: usize,
    pub column: usize,
    pub columns: usize,
    pub shape: CursorShape,
    pub character: char,
}
