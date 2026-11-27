use crate::frame::dock_side::DockSide;
use crate::frame::metrics::{
    COLLAPSE_BELOW, DEFAULT_WIDTH, MAX_WIDTH, MIN_WIDTH, NARROW_BELOW, WINDOW_SHARE,
};
use crate::frame::presentation::Presentation;
use crate::frame::visibility::Visibility;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Panel {
    visibility: Visibility,
    preferred: f32,
    side: DockSide,
}

impl Default for Panel {
    fn default() -> Self {
        Self {
            visibility: Visibility::Open,
            preferred: DEFAULT_WIDTH,
            side: DockSide::Left,
        }
    }
}

impl Panel {
    pub fn restore(
        visibility: Visibility,
        preferred: f32,
        side: DockSide,
    ) -> Self {
        Self {
            visibility,
            preferred: preferred.clamp(MIN_WIDTH, MAX_WIDTH),
            side,
        }
    }

    pub fn visibility(self) -> Visibility {
        self.visibility
    }

    pub fn preferred(self) -> f32 {
        self.preferred
    }

    pub fn side(self) -> DockSide {
        self.side
    }

    pub fn dock_to(
        &mut self,
        side: DockSide,
    ) {
        self.side = side;
    }

    pub fn toggle(&mut self) {
        self.visibility = self.visibility.flipped();
    }

    pub(crate) fn max_width(container: f32) -> f32 {
        MAX_WIDTH
            .min((container - 1.0) * WINDOW_SHARE)
            .max(MIN_WIDTH)
    }

    pub fn track_width(
        self,
        container: f32,
    ) -> f32 {
        match self.visibility {
            Visibility::Closed => 0.0,
            Visibility::Open => self.preferred.min(container * WINDOW_SHARE),
        }
    }

    pub(crate) fn settle(
        width: f32,
        container: f32,
    ) -> f32 {
        if width < COLLAPSE_BELOW {
            return 0.0;
        }
        width.clamp(MIN_WIDTH, Self::max_width(container))
    }

    pub fn commit(
        &mut self,
        width: f32,
    ) {
        if width <= 0.0 {
            self.visibility = Visibility::Closed;
            return;
        }
        self.visibility = Visibility::Open;
        self.preferred = width.round().clamp(MIN_WIDTH, MAX_WIDTH);
    }

    pub fn presentation(container: f32) -> Presentation {
        if container < NARROW_BELOW {
            return Presentation::Overlay;
        }
        Presentation::Docked
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn assert_close(
        actual: f32,
        expected: f32,
        what: &str,
    ) {
        assert!(
            (actual - expected).abs() < 0.01,
            "{what}: {actual} != {expected}"
        );
    }

    #[test]
    fn restore_clamps_the_stored_width() {
        for (stored, expected) in [(100.0, 250.0), (294.0, 294.0), (900.0, 640.0)] {
            let panel = Panel::restore(Visibility::Open, stored, DockSide::Left);
            assert_close(panel.preferred(), expected, "restored preference");
        }
    }

    #[test]
    fn closed_track_has_no_width_and_window_pressure_does_not_touch_the_preference() {
        let mut panel = Panel::default();
        assert_close(panel.track_width(1200.0), 294.0, "open track");
        assert_close(panel.track_width(300.0), 240.0, "squeezed to 80%");
        assert_close(panel.preferred(), 294.0, "preference survives pressure");
        panel.toggle();
        assert_close(panel.track_width(1200.0), 0.0, "closed track");
    }

    #[test]
    fn settle_collapses_below_half_the_minimum_and_clamps_otherwise() {
        let cases = [
            (124.0, 1200.0, 0.0),
            (125.0, 1200.0, 250.0),
            (400.0, 1200.0, 400.0),
            (900.0, 1200.0, 640.0),
            (900.0, 700.0, 559.2),
        ];
        for (width, container, expected) in cases {
            assert_close(Panel::settle(width, container), expected, "settled width");
        }
    }

    #[test]
    fn commit_zero_closes_and_keeps_the_old_preference() {
        let mut panel = Panel::default();
        panel.commit(0.0);
        assert_eq!(panel.visibility(), Visibility::Closed, "zero closes");
        assert_close(panel.preferred(), 294.0, "preference kept");
        panel.commit(420.4);
        assert_eq!(panel.visibility(), Visibility::Open, "a width reopens");
        assert_close(panel.preferred(), 420.0, "rounded preference");
    }

    #[test]
    fn narrow_windows_use_the_drawer() {
        assert_eq!(Panel::presentation(639.0), Presentation::Overlay, "narrow");
        assert_eq!(Panel::presentation(640.0), Presentation::Docked, "wide");
    }
}
