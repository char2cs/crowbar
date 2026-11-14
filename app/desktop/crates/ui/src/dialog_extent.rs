//! A dialog that fills a share of the window instead of fitting its content.

use gpui::{Pixels, Size, px, size};

const GUTTER: f32 = 32.0;

/// The size of a [`crate::Dialog`] as shares of the window, each capped; also removes the body padding and the header rule.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct DialogExtent {
    width_share: f32,
    height_share: f32,
    max_width: Pixels,
    max_height: Pixels,
}

impl DialogExtent {
    /// An extent of `width_share` x `height_share` of the window, at most `max_width` x `max_height`.
    pub fn new(width_share: f32, height_share: f32, max_width: Pixels, max_height: Pixels) -> Self {
        Self {
            width_share,
            height_share,
            max_width,
            max_height,
        }
    }

    /// The dialog size in a window of `viewport`: the shares, capped by the maximum and by the viewport minus a gutter.
    pub fn size(self, viewport: Size<Pixels>) -> Size<Pixels> {
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
#[path = "dialog_extent_tests.rs"]
mod tests;
