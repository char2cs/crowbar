use crate::frame::metrics::{MAX_WIDTH, MIN_WIDTH};
use crate::frame::panel::Panel;
use crate::frame::separator::key_step::KeyStep;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ResizeKey {
    Left,
    Right,
    Home,
    End,
    Enter,
}

impl ResizeKey {
    pub fn apply(
        self,
        step: KeyStep,
        panel: Panel,
        (current, container): (f32, f32),
    ) -> f32 {
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

fn toggled(
    current: f32,
    panel: Panel,
    container: f32,
) -> f32 {
    if current > 0.0 {
        return 0.0;
    }
    panel.preferred().min(Panel::max_width(container))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::frame::dock_side::DockSide;

    fn width(
        key: ResizeKey,
        step: KeyStep,
        (current, side): (f32, DockSide),
    ) -> f32 {
        let mut panel = Panel::default();
        panel.dock_to(side);
        key.apply(step, panel, (current, 1200.0))
    }

    type KeyCase = (&'static str, ResizeKey, KeyStep, (f32, DockSide), f32);

    const KEY_CASES: &[KeyCase] = &[
        (
            "left dock grows",
            ResizeKey::Right,
            KeyStep::Fine,
            (300.0, DockSide::Left),
            316.0,
        ),
        (
            "left dock shrinks",
            ResizeKey::Left,
            KeyStep::Fine,
            (300.0, DockSide::Left),
            284.0,
        ),
        (
            "right dock grows",
            ResizeKey::Left,
            KeyStep::Fine,
            (300.0, DockSide::Right),
            316.0,
        ),
        (
            "shift steps by sixty four",
            ResizeKey::Right,
            KeyStep::Coarse,
            (300.0, DockSide::Left),
            364.0,
        ),
        (
            "clamped at the minimum",
            ResizeKey::Left,
            KeyStep::Fine,
            (250.0, DockSide::Left),
            250.0,
        ),
        (
            "stays collapsed",
            ResizeKey::Left,
            KeyStep::Fine,
            (0.0, DockSide::Left),
            0.0,
        ),
        (
            "home collapses",
            ResizeKey::Home,
            KeyStep::Fine,
            (300.0, DockSide::Left),
            0.0,
        ),
        (
            "end maximises",
            ResizeKey::End,
            KeyStep::Fine,
            (300.0, DockSide::Left),
            640.0,
        ),
        (
            "enter collapses",
            ResizeKey::Enter,
            KeyStep::Fine,
            (300.0, DockSide::Left),
            0.0,
        ),
        (
            "enter reopens",
            ResizeKey::Enter,
            KeyStep::Fine,
            (0.0, DockSide::Left),
            294.0,
        ),
    ];

    #[test]
    fn keys_move_clamp_collapse_and_toggle_the_width() {
        for (name, key, step, from, expected) in KEY_CASES {
            let moved = width(*key, *step, *from);
            assert!((moved - expected).abs() < 0.01, "{name}: got {moved}");
        }
    }
}
