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
