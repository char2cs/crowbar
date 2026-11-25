/// The edge of a native menu that lines up with the point it opens at.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum MenuEdge {
    /// Its left edge, as a menu at the pointer opens.
    #[default]
    Start,
    /// Its right edge, as a menu that ends with its trigger opens.
    End,
}
