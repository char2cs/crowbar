//! Geometry of the scrollbar thumb along one axis.

const MARGIN: f32 = 4.0;
const MIN_LENGTH: f32 = 24.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct Thumb {
    pub(crate) start: f32,
    pub(crate) length: f32,
}

impl Thumb {
    pub(crate) fn locate(
        view: f32,
        max_offset: f32,
        scrolled: f32,
    ) -> Option<Self> {
        if max_offset <= 0.0 || view <= 2.0 * MARGIN {
            return None;
        }
        let track = view - 2.0 * MARGIN;
        let length = (track * view / (view + max_offset)).clamp(MIN_LENGTH.min(track), track);
        let progress = (scrolled / max_offset).clamp(0.0, 1.0);
        Some(Self {
            start: MARGIN + (track - length) * progress,
            length,
        })
    }

    pub(crate) fn scrolled_after(
        view: f32,
        max_offset: f32,
        scrolled: f32,
        delta: f32,
    ) -> f32 {
        let Some(thumb) = Self::locate(view, max_offset, scrolled) else {
            return scrolled;
        };
        let travel = view - 2.0 * MARGIN - thumb.length;
        if travel <= 0.0 {
            return scrolled;
        }
        (scrolled + delta / travel * max_offset).clamp(0.0, max_offset)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Args = (f32, f32, f32);

    type LocateCase = (&'static str, Args, Option<(f32, f32)>);

    const LOCATE: &[LocateCase] = &[
        ("content that fits has no thumb", (300.0, 0.0, 0.0), None),
        (
            "a track shorter than its margins has no thumb",
            (6.0, 100.0, 0.0),
            None,
        ),
        (
            "half the content is half the track",
            (300.0, 300.0, 0.0),
            Some((146.0, 4.0)),
        ),
        (
            "the length has a floor",
            (300.0, 30_000.0, 0.0),
            Some((24.0, 4.0)),
        ),
        (
            "at the end the thumb meets the margin",
            (300.0, 300.0, 300.0),
            Some((146.0, 150.0)),
        ),
    ];

    type ScrolledCase = (&'static str, (f32, f32, f32, f32), f32);

    const SCROLLED: &[ScrolledCase] = &[
        (
            "half the travel is half the offset",
            (300.0, 300.0, 0.0, 73.0),
            150.0,
        ),
        (
            "the offset is capped at the end",
            (300.0, 300.0, 100.0, 1.0e6),
            300.0,
        ),
        (
            "content that fits never scrolls",
            (300.0, 0.0, 0.0, 10.0),
            0.0,
        ),
    ];

    #[test]
    fn the_thumb_is_proportional_with_a_floor_and_margins() {
        for &(name, (track, content, offset), want) in LOCATE {
            let got =
                Thumb::locate(track, content, offset).map(|thumb| (thumb.length, thumb.start));
            let close = |a: f32, b: f32| (a - b).abs() < 0.01;
            let same = match (got, want) {
                (Some(got), Some(want)) => close(got.0, want.0) && close(got.1, want.1),
                (got, want) => got == want,
            };
            assert!(same, "{name}: {got:?} vs {want:?}");
        }
    }

    #[test]
    fn dragging_maps_thumb_travel_to_scroll_offset() {
        for &(name, (track, content, offset, travel), want) in SCROLLED {
            let after = Thumb::scrolled_after(track, content, offset, travel);
            assert!((after - want).abs() < 0.01, "{name}: {after}");
        }
    }
}
