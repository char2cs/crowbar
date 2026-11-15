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
mod tests {
    use super::*;

    #[test]
    fn end_and_middle_follow_from_x_and_width() {
        let span = Span {
            x: 10.0,
            width: 40.0,
        };
        assert_eq!(span.end(), 50.0, "right edge");
        assert_eq!(span.middle(), 30.0, "centre");
    }
}
