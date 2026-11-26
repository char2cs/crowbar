use serde::{Deserialize, Serialize};

use crate::placement::Placement;
use crate::rect::Rect;

pub(crate) const MIN_SIZE: (f32, f32) = (640.0, 480.0);

/// The size of a first-run window, in logical pixels (width, height).
pub const DEFAULT_SIZE: (f32, f32) = (1280.0, 800.0);

const VISIBLE_FRACTION: f32 = 0.25;

/// What is remembered of the window: its normal bounds and whether it was maximized.
#[derive(Debug, Clone, Copy, PartialEq, Serialize, Deserialize)]
pub struct WindowState {
    /// The bounds the window has when not maximized.
    pub bounds: Rect,
    /// The window was maximized when the app last closed.
    pub maximized: bool,
}

impl WindowState {
    /// Where to open given the connected `displays`. The bounds are grown to at least 640×480; if
    /// less than a quarter of the window is on any display, it moves onto the first display
    /// (the primary one); with no displays known the remembered bounds are used as they are.
    #[must_use]
    pub fn placement(
        &self,
        displays: &[Rect],
    ) -> Placement {
        let (widest, tallest) = displays
            .iter()
            .fold((0.0_f32, 0.0_f32), |(width, height), display| {
                (width.max(display.width), height.max(display.height))
            });
        let bounds = Rect {
            width: capped(self.bounds.width, widest).max(MIN_SIZE.0),
            height: capped(self.bounds.height, tallest).max(MIN_SIZE.1),
            ..self.bounds
        };
        Placement {
            bounds: on_a_display(bounds, displays),
            maximized: self.maximized,
        }
    }
}

fn capped(
    value: f32,
    limit: f32,
) -> f32 {
    if limit <= 0.0 {
        return value;
    }
    value.min(limit)
}

fn on_a_display(
    bounds: Rect,
    displays: &[Rect],
) -> Rect {
    let visible = displays
        .iter()
        .map(|display| bounds.overlap_area(display))
        .fold(0.0, f32::max);
    let enough = visible >= bounds.width * bounds.height * VISIBLE_FRACTION;
    let Some(primary) = displays.first() else {
        return bounds;
    };
    if enough {
        return bounds;
    }
    Rect {
        x: primary.x + ((primary.width - bounds.width) / 2.0).max(0.0),
        y: primary.y + ((primary.height - bounds.height) / 2.0).max(0.0),
        ..bounds
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const DISPLAY: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 1920.0,
        height: 1080.0,
    };

    const fn state(
        x: f32,
        y: f32,
        width: f32,
        height: f32,
    ) -> WindowState {
        let bounds = Rect {
            x,
            y,
            width,
            height,
        };
        WindowState {
            bounds,
            maximized: false,
        }
    }

    #[test]
    fn size_and_maximized_follow_the_saved_state() {
        const SMALL: WindowState = state(0.0, 0.0, 100.0, 100.0);
        type Case<'a> = (WindowState, &'a [Rect], (f32, f32), bool);
        let cases: [Case<'_>; 4] = [
            (
                state(100.0, 80.0, 1000.0, 700.0),
                &[DISPLAY],
                (1000.0, 700.0),
                false,
            ),
            (SMALL, &[DISPLAY], MIN_SIZE, false),
            (
                state(0.0, 0.0, 1e30, 5000.0),
                &[DISPLAY],
                (DISPLAY.width, DISPLAY.height),
                false,
            ),
            (
                state(4000.0, 0.0, 1000.0, 700.0),
                &[],
                (1000.0, 700.0),
                true,
            ),
        ];
        for (mut saved, displays, size, maximized) in cases {
            saved.maximized = maximized;
            let placement = saved.placement(displays);
            assert!(
                (placement.bounds.width - size.0).abs() < f32::EPSILON,
                "{saved:?}"
            );
            assert!(
                (placement.bounds.height - size.1).abs() < f32::EPSILON,
                "{saved:?}"
            );
            assert_eq!(placement.maximized, maximized);
        }
    }

    #[test]
    fn a_window_stays_on_a_display_or_is_centred_on_the_primary() {
        type Case<'a> = (WindowState, &'a [Rect], (f32, f32));
        let cases: [Case<'_>; 3] = [
            (state(100.0, 80.0, 1000.0, 700.0), &[DISPLAY], (100.0, 80.0)),
            (
                state(4000.0, 0.0, 1000.0, 700.0),
                &[DISPLAY],
                (460.0, 190.0),
            ),
            (state(4000.0, 0.0, 1000.0, 700.0), &[], (4000.0, 0.0)),
        ];
        for (saved, displays, (x, y)) in cases {
            let placement = saved.placement(displays);
            assert!((placement.bounds.x - x).abs() < f32::EPSILON, "{saved:?}");
            assert!((placement.bounds.y - y).abs() < f32::EPSILON, "{saved:?}");
        }
    }
}
