#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub(crate) struct Span {
    pub x: f32,
    pub width: f32,
}

impl Span {
    #[must_use]
    pub(crate) fn end(self) -> f32 {
        self.x + self.width
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn end_follows_from_x_and_width() {
        let span = Span {
            x: 10.0,
            width: 40.0,
        };
        assert_eq!(span.end(), 50.0, "right edge");
    }
}
