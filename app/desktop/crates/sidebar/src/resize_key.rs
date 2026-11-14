//! Keyboard control of the focused separator.

use crate::key_step::KeyStep;
use crate::metrics::{MAX_WIDTH, MIN_WIDTH};
use crate::panel::Panel;

/// A key the focused separator reacts to.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ResizeKey {
    /// Left arrow.
    Left,
    /// Right arrow.
    Right,
    /// Home: collapse.
    Home,
    /// End: widest.
    End,
    /// Enter: toggle between collapsed and the preference.
    Enter,
}

impl ResizeKey {
    /// The width after pressing this key with the column `current` pixels wide in a container
    /// `container` pixels wide, both given as `(current, container)`; zero collapses.
    /// Arrow directions are mirrored for a right dock, results below zero collapse and every
    /// other result clamps.
    pub fn apply(self, step: KeyStep, panel: Panel, (current, container): (f32, f32)) -> f32 {
        let sign = panel.side().growth_sign();
        let pixels = step.pixels();
        let target = match self {
            Self::Left => current - sign * pixels,
            Self::Right => current + sign * pixels,
            Self::Home => return 0.0,
            Self::End => Panel::max_width(container),
            Self::Enter => return toggled(current, panel, container),
        };
        if target < 0.0 {
            return 0.0;
        }
        target.clamp(MIN_WIDTH, Panel::max_width(container).min(MAX_WIDTH))
    }
}

fn toggled(current: f32, panel: Panel, container: f32) -> f32 {
    if current > 0.0 {
        return 0.0;
    }
    panel.preferred().min(Panel::max_width(container))
}

#[cfg(test)]
#[path = "resize_key_tests.rs"]
mod tests;
