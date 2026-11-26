/// Which way a line command moves or copies lines.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Vertical {
    /// Toward the first line.
    Up,
    /// Toward the last line.
    Down,
}
