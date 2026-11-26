/// The edge of a native menu that lines up with the point it opens at.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum MenuEdge {
    /// Its left edge.
    #[default]
    Start,
    /// Its right edge.
    End,
}
