use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub struct Rect {
    pub x: f32,
    pub y: f32,
    pub width: f32,
    pub height: f32,
}

impl Rect {
    pub(crate) fn is_valid(self) -> bool {
        [self.x, self.y, self.width, self.height]
            .iter()
            .all(|v| v.is_finite())
            && self.width >= 0.0
            && self.height >= 0.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const fn rect(
        x: f32,
        y: f32,
        width: f32,
        height: f32,
    ) -> Rect {
        Rect {
            x,
            y,
            width,
            height,
        }
    }

    #[test]
    fn only_finite_rectangles_with_a_non_negative_size_are_valid() {
        assert!(
            rect(-10.0, 5.0, 0.0, 0.0).is_valid(),
            "origin may be negative, size may be zero"
        );
        let invalid = [
            rect(f32::NAN, 0.0, 1.0, 1.0),
            rect(0.0, f32::INFINITY, 1.0, 1.0),
            rect(0.0, 0.0, f32::NAN, 1.0),
            rect(0.0, 0.0, 1.0, f32::NEG_INFINITY),
            rect(0.0, 0.0, -1.0, 1.0),
            rect(0.0, 0.0, 1.0, -1.0),
        ];
        for bad in invalid {
            assert!(!bad.is_valid(), "{bad:?}");
        }
    }
}
