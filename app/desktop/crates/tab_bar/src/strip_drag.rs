use model::{Command, TabId};

use crate::span::Span;

const HYSTERESIS: f32 = 6.0;
const BAND_MARGIN: f32 = 32.0;

/// A tab being dragged along the strip: it follows the pointer, the others slide aside, and
/// releasing drops it into the gap. Pure geometry; the strip view feeds it pointer positions.
#[derive(Debug, Clone, PartialEq)]
pub struct StripDrag {
    tabs: Vec<TabId>,
    spans: Vec<Span>,
    dragged: usize,
    grab: f32,
    target: usize,
}

impl StripDrag {
    /// Starts dragging the tab at `dragged` among `tabs` laid out in `spans`, grabbed at
    /// pointer `x`; the slots must be the strip's content coordinates, scroll included.
    #[must_use]
    pub fn begin(tabs: Vec<TabId>, spans: Vec<Span>, dragged: usize, x: f32) -> Option<Self> {
        let slot = spans.get(dragged)?;
        (tabs.len() == spans.len()).then_some(Self {
            grab: x - slot.x,
            tabs,
            spans,
            dragged,
            target: dragged,
        })
    }

    /// The index the tab would land on if released now.
    #[must_use]
    pub fn target(&self) -> usize {
        self.target
    }

    /// Left edge of the dragged tab for pointer `x`, held between the first and last slot.
    #[must_use]
    pub fn ghost_x(&self, x: f32) -> f32 {
        let (Some(first), Some(last)) = (self.spans.first(), self.spans.last()) else {
            return x;
        };
        let width = self.width();
        (x - self.grab).clamp(first.x, (last.end() - width).max(first.x))
    }

    /// Top of the ghost: pinned to the strip row while the pointer is near it, easing toward the
    /// free pointer across 32px so leaving the band has no jump.
    #[must_use]
    pub fn ghost_y(&self, pointer_y: f32, row_top: f32, grab_y: f32) -> f32 {
        let delta = pointer_y - grab_y - row_top;
        row_top + delta * (delta.abs() / BAND_MARGIN).clamp(0.0, 1.0)
    }

    /// Moves the pointer to `x`; the target changes hands once the ghost has passed a
    /// neighbour's middle by 6px.
    pub fn moved(&mut self, x: f32) {
        let ghost = self.ghost_x(x);
        let width = self.width();
        while self.step(ghost, width) {}
    }

    /// How far slot `index` is shifted to make room: the dragged tab's width for the tabs it has
    /// passed, zero for the rest and for the dragged tab itself.
    #[must_use]
    pub fn offset(&self, index: usize) -> f32 {
        let width = self.width();
        if index > self.dragged && index <= self.target {
            return -width;
        }
        if index < self.dragged && index >= self.target {
            return width;
        }
        0.0
    }

    /// The command that performs the drop, or `None` when the tab is back where it started.
    #[must_use]
    pub fn drop(&self) -> Option<Command> {
        let tab = *self.tabs.get(self.dragged)?;
        (self.target != self.dragged).then_some(Command::MoveTab {
            tab,
            to_index: self.target,
        })
    }

    /// The tab being dragged.
    #[must_use]
    pub fn tab(&self) -> Option<TabId> {
        self.tabs.get(self.dragged).copied()
    }

    fn width(&self) -> f32 {
        self.spans.get(self.dragged).map_or(0.0, |span| span.width)
    }

    fn step(&mut self, ghost: f32, width: f32) -> bool {
        let Some(home) = self.spans.get(self.target) else {
            return false;
        };
        let reach = width / 2.0 + HYSTERESIS;
        if ghost > home.x + reach && self.target + 1 < self.spans.len() {
            self.target += 1;
            return true;
        }
        if ghost < home.x - reach && self.target > 0 {
            self.target -= 1;
            return true;
        }
        false
    }
}

#[cfg(test)]
#[path = "strip_drag_tests.rs"]
mod tests;
