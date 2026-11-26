use layout::{Edge, Rect};

const BAND: f32 = 0.25;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum PaneZone {
    Left,
    Right,
    Top,
    Bottom,
    Center,
}

impl PaneZone {
    pub(crate) fn at(
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

    pub(crate) fn edge(self) -> Edge {
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
    use super::*;

    #[test]
    fn every_point_of_a_pane_falls_in_its_zone() {
        let pane = Rect {
            x: 100.0,
            y: 50.0,
            width: 400.0,
            height: 200.0,
        };
        let cases = [
            ((110.0, 150.0), PaneZone::Left),
            ((490.0, 150.0), PaneZone::Right),
            ((300.0, 55.0), PaneZone::Top),
            ((300.0, 245.0), PaneZone::Bottom),
            ((300.0, 150.0), PaneZone::Center),
            ((101.0, 51.0), PaneZone::Left),
            ((120.0, 52.0), PaneZone::Top),
            ((499.0, 249.0), PaneZone::Right),
            ((130.0, 248.0), PaneZone::Bottom),
            ((199.0, 150.0), PaneZone::Left),
            ((201.0, 150.0), PaneZone::Center),
        ];
        for (point, zone) in cases {
            assert_eq!(PaneZone::at(point, pane), zone, "zone of {point:?}");
        }
    }
}
