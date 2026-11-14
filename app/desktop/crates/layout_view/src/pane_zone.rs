use model::{Edge, Rect};

const BAND: f32 = 0.25;

/// The region of a pane a pointer is over while a chat is dragged across it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PaneZone {
    /// The left quarter, outside the top and bottom corner triangles.
    Left,
    /// The right quarter.
    Right,
    /// The top quarter.
    Top,
    /// The bottom quarter.
    Bottom,
    /// Everything else.
    Center,
}

impl PaneZone {
    /// The zone of `point` inside `rect`. Left and right win the corners inside their triangles;
    /// top and bottom own the rest of their bands.
    #[must_use]
    pub fn at(point: (f32, f32), rect: Rect) -> Self {
        let nx = (point.0 - rect.x) / rect.width;
        let ny = (point.1 - rect.y) / rect.height;
        if nx < BAND && nx < ny && nx < 1.0 - ny {
            return Self::Left;
        }
        if nx > 1.0 - BAND && 1.0 - nx < ny && 1.0 - nx < 1.0 - ny {
            return Self::Right;
        }
        if ny < BAND {
            return Self::Top;
        }
        if ny > 1.0 - BAND {
            return Self::Bottom;
        }
        Self::Center
    }

    /// The side a chat dropped here lands on. The centre of an open pane adds the chat beside
    /// it on the right, and the preview shows exactly that.
    #[must_use]
    pub fn edge(self) -> Edge {
        match self {
            Self::Left => Edge::Left,
            Self::Top => Edge::Top,
            Self::Bottom => Edge::Bottom,
            Self::Right | Self::Center => Edge::Right,
        }
    }
}

#[cfg(test)]
#[path = "pane_zone_tests.rs"]
mod tests;
