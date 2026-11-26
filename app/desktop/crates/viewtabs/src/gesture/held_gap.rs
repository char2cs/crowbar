use std::time::Instant;

use gpui::{Pixels, Point};
use ui::{CubicBezier, Glide, Motion, timing};

use crate::span::Span;
use crate::strip_metrics::SHELL_MARGIN;

const HYSTERESIS: f32 = 6.0;

#[derive(Debug)]
pub(crate) struct HeldGap {
    pub(crate) pointer: Point<Pixels>,
    pitch: f32,
    target: Option<usize>,
    slides: Vec<Glide>,
}

impl HeldGap {
    pub(crate) fn new(
        pointer: Point<Pixels>,
        pitch: f32,
        tabs: usize,
    ) -> Self {
        let slide = Glide::resting(0.0, timing::DEFAULT, CubicBezier::EASE_OUT);
        Self {
            pointer,
            pitch,
            target: None,
            slides: vec![slide; tabs],
        }
    }

    pub(crate) fn target(&self) -> Option<usize> {
        self.target
    }

    pub(crate) fn width(&self) -> f32 {
        self.pitch - 2.0 * SHELL_MARGIN
    }

    pub(crate) fn aim(
        &mut self,
        x: Option<f32>,
        spans: &[Span],
        visible: Span,
    ) {
        let Some(x) = x else {
            self.target = None;
            return;
        };
        let low = self.index_at(x - HYSTERESIS, spans, visible);
        let high = self.index_at(x + HYSTERESIS, spans, visible);
        let current = self.target.unwrap_or(spans.len());
        self.target = Some(current.clamp(low, high));
    }

    pub(crate) fn slide_to(
        &mut self,
        now: Instant,
        motion: Motion,
    ) {
        let (to, pitch) = (self.target.unwrap_or(usize::MAX), self.pitch);
        for (index, slide) in self.slides.iter_mut().enumerate() {
            slide.go_to(pitch * f32::from(u8::from(index >= to)), now, motion);
        }
    }

    pub(crate) fn slide(
        &self,
        index: usize,
        now: Instant,
        motion: Motion,
    ) -> f32 {
        self.slides
            .get(index)
            .map_or(0.0, |slide| slide.value(now, motion))
    }

    pub(crate) fn is_sliding(
        &self,
        now: Instant,
        motion: Motion,
    ) -> bool {
        self.slides
            .iter()
            .any(|slide| slide.is_running(now, motion))
    }

    pub(crate) fn pinned_left(
        &self,
        x: f32,
        spans: &[Span],
        visible: Span,
    ) -> f32 {
        let (Some(first), Some(last)) = (spans.first(), spans.last()) else {
            return x;
        };
        let half = self.width() / 2.0;
        let low = first.x.max(visible.x - half) + SHELL_MARGIN;
        let high = last.end().min(visible.end() - half) + SHELL_MARGIN;
        (x - half).min(high).max(low)
    }

    fn index_at(
        &self,
        x: f32,
        spans: &[Span],
        visible: Span,
    ) -> usize {
        let centre = self.pinned_left(x, spans, visible) + self.width() / 2.0;
        spans.iter().filter(|span| span.end() <= centre).count()
    }
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use gpui::{point, px};
    use ui::Motion;

    use super::*;

    const PITCH: f32 = 200.0;

    fn spans() -> Vec<Span> {
        (0..2u16)
            .map(|i| Span {
                x: f32::from(i) * 240.0,
                width: 240.0,
            })
            .collect()
    }

    fn visible() -> Span {
        Span {
            x: 0.0,
            width: 600.0,
        }
    }

    fn gap() -> HeldGap {
        HeldGap::new(point(px(0.0), px(0.0)), PITCH, 2)
    }

    struct Case {
        name: &'static str,
        held: &'static [f32],
        target: usize,
    }

    const CASES: &[Case] = &[
        Case {
            name: "over tab one",
            held: &[100.0],
            target: 0,
        },
        Case {
            name: "over tab two",
            held: &[250.0],
            target: 1,
        },
        Case {
            name: "past the last tab",
            held: &[590.0],
            target: 2,
        },
        Case {
            name: "inside the six pixels past a tab end",
            held: &[250.0, 243.0],
            target: 1,
        },
        Case {
            name: "past the six pixels",
            held: &[250.0, 233.0],
            target: 0,
        },
    ];

    fn held_at(xs: &[f32]) -> HeldGap {
        let mut gap = gap();
        for x in xs {
            gap.aim(Some(*x), &spans(), visible());
        }
        gap
    }

    #[test]
    fn the_gap_opens_before_every_tab_the_held_centre_has_passed_by_six_pixels() {
        for case in CASES {
            let gap = held_at(case.held);
            assert_eq!(gap.target(), Some(case.target), "{}", case.name);
        }
    }

    #[test]
    fn the_tabs_from_the_gap_on_slide_by_the_joined_pitch_over_150_ms() {
        let (start, motion) = (Instant::now(), Motion::default());
        let end = start + Duration::from_millis(150);
        let mut gap = gap();
        gap.aim(Some(250.0), &spans(), visible());
        gap.slide_to(start, motion);
        assert_eq!(gap.slide(1, start, motion), 0.0, "starts home");
        assert!(gap.is_sliding(start, motion), "sliding");
        assert_eq!(gap.slide(0, end, motion), 0.0, "before the gap");
        assert_eq!(gap.slide(1, end, motion), PITCH, "after it");
        gap.aim(None, &spans(), visible());
        gap.slide_to(end, motion);
        let back = end + Duration::from_millis(150);
        assert_eq!(gap.slide(1, back, motion), 0.0, "closed again");
        assert_eq!(gap.target(), None, "no drop once released");
    }

    #[test]
    fn the_held_tab_stays_over_the_tabs_and_their_one_new_slot() {
        let gap = gap();
        assert_eq!(gap.pinned_left(250.0, &spans(), visible()), 152.0, "free");
        assert_eq!(gap.pinned_left(10.0, &spans(), visible()), 2.0, "first");
        assert_eq!(gap.pinned_left(900.0, &spans(), visible()), 482.0, "last");
    }
}
