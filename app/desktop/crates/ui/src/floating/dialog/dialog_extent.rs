use gpui::{Pixels, Size, px, size};

const GUTTER: f32 = 32.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct DialogExtent {
    width_share: f32,
    height_share: f32,
    max_width: Pixels,
    max_height: Pixels,
}

impl DialogExtent {
    pub fn new(
        width_share: f32,
        height_share: f32,
        max_width: Pixels,
        max_height: Pixels,
    ) -> Self {
        Self {
            width_share,
            height_share,
            max_width,
            max_height,
        }
    }

    pub fn size(
        self,
        viewport: Size<Pixels>,
    ) -> Size<Pixels> {
        let room = |total: Pixels| (total - px(GUTTER)).max(px(0.0));
        size(
            (viewport.width * self.width_share)
                .min(self.max_width)
                .min(room(viewport.width)),
            (viewport.height * self.height_share)
                .min(self.max_height)
                .min(room(viewport.height)),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shares_are_capped_and_leave_the_gutter() {
        type Viewport = (f32, f32);
        const CASES: &[(Viewport, Viewport)] = &[
            ((1200.0, 800.0), (1080.0, 592.0)),
            ((3000.0, 2000.0), (1120.0, 820.0)),
            ((400.0, 300.0), (360.0, 222.0)),
        ];
        let extent = DialogExtent::new(0.9, 0.74, px(1120.0), px(820.0));
        for &((width, height), (expected_width, expected_height)) in CASES {
            let got = extent.size(size(px(width), px(height)));
            assert_eq!(
                (f32::from(got.width), f32::from(got.height)),
                (expected_width, expected_height),
                "{width}x{height}"
            );
        }
    }
}
