use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub struct Rect {
    pub x: f32,
    pub y: f32,
    pub width: f32,
    pub height: f32,
}

impl Rect {
    pub(crate) fn validated(self) -> Option<Self> {
        let finite = [self.x, self.y, self.width, self.height]
            .iter()
            .all(|value| value.is_finite());
        let positive = self.width > 0.0 && self.height > 0.0;
        (finite && positive).then_some(self)
    }

    pub(crate) fn overlap_area(
        &self,
        other: &Self,
    ) -> f32 {
        let width = (self.x + self.width).min(other.x + other.width) - self.x.max(other.x);
        let height = (self.y + self.height).min(other.y + other.height) - self.y.max(other.y);
        width.max(0.0) * height.max(0.0)
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
    fn non_finite_or_empty_rects_are_not_valid() {
        const CASES: &[(f32, f32, f32)] = &[
            (f32::NAN, 100.0, 100.0),
            (0.0, 100.0, f32::INFINITY),
            (0.0, 0.0, 100.0),
        ];
        let valid = rect(0.0, 0.0, 100.0, 100.0);
        assert_eq!(valid.validated(), Some(valid));
        for &(x, width, height) in CASES {
            assert_eq!(rect(x, 0.0, width, height).validated(), None);
        }
    }
}
