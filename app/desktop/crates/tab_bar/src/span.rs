/// A horizontal extent: where it starts and how wide it is.
#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub struct Span {
    /// Left edge.
    pub x: f32,
    /// Width.
    pub width: f32,
}

impl Span {
    /// The right edge.
    #[must_use]
    pub fn end(self) -> f32 {
        self.x + self.width
    }

    /// The middle.
    #[must_use]
    pub fn middle(self) -> f32 {
        self.x + self.width / 2.0
    }
}

#[cfg(test)]
#[path = "span_tests.rs"]
mod tests;
