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
    pub fn at(
        point: (f32, f32),
        rect: Rect,
    ) -> Self {
        let nx = (point.0 - rect.x) / rect.width;
        let ny = (point.1 - rect.y) / rect.height;
        let in_left_triangle = nx < ny && nx < 1.0 - ny;
        if nx < BAND && in_left_triangle {
            return Self::Left;
        }
        let in_right_triangle = 1.0 - nx < ny && 1.0 - nx < 1.0 - ny;
        if nx > 1.0 - BAND && in_right_triangle {
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
mod tests {
    use model::{Edge, Rect};

    use super::*;

    const PANE: Rect = Rect {
        x: 100.0,
        y: 50.0,
        width: 400.0,
        height: 200.0,
    };

    type Case = ((f32, f32), PaneZone, Edge);

    const CASES: &[Case] = &[
        ((110.0, 150.0), PaneZone::Left, Edge::Left),
        ((490.0, 150.0), PaneZone::Right, Edge::Right),
        ((300.0, 55.0), PaneZone::Top, Edge::Top),
        ((300.0, 245.0), PaneZone::Bottom, Edge::Bottom),
        ((300.0, 150.0), PaneZone::Center, Edge::Right),
        ((101.0, 51.0), PaneZone::Left, Edge::Left),
        ((120.0, 52.0), PaneZone::Top, Edge::Top),
        ((499.0, 249.0), PaneZone::Right, Edge::Right),
        ((130.0, 248.0), PaneZone::Bottom, Edge::Bottom),
    ];

    #[test]
    fn every_point_resolves_to_its_zone_and_edge() {
        for (point, zone, edge) in CASES {
            let found = PaneZone::at(*point, PANE);
            assert_eq!(found, *zone, "zone of {point:?}");
            assert_eq!(found.edge(), *edge, "edge of {point:?}");
        }
    }

    #[test]
    fn the_quarter_bands_end_exactly_at_a_quarter() {
        assert_eq!(
            PaneZone::at((199.0, 150.0), PANE),
            PaneZone::Left,
            "just inside"
        );
        assert_eq!(
            PaneZone::at((201.0, 150.0), PANE),
            PaneZone::Center,
            "just outside"
        );
    }
}
