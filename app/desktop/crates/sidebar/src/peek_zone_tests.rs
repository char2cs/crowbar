use super::*;

fn left() -> PeekZone {
    PeekZone {
        window: (1200.0, 800.0),
        card_width: 294.0,
        side: DockSide::Left,
    }
}

#[test]
fn the_band_starts_below_the_chrome_row_and_ends_above_the_bottom_corner() {
    let zone = left();
    assert!(zone.summons(0.0, 42.0), "top of the band");
    assert!(zone.summons(6.0, 784.0), "bottom of the band");
    assert!(!zone.summons(0.0, 41.0), "chrome row");
    assert!(!zone.summons(0.0, 785.0), "bottom corner");
    assert!(!zone.summons(7.0, 400.0), "outside the band");
}

#[test]
fn the_right_dock_mirrors_the_band() {
    let zone = PeekZone {
        side: DockSide::Right,
        ..left()
    };
    assert!(zone.summons(1194.0, 400.0), "inside");
    assert!(!zone.summons(1100.0, 400.0), "outside");
}

#[test]
fn the_card_keeps_the_pointer_within_eight_pixels_of_its_edge() {
    let zone = left();
    assert!(zone.keeps(310.0, 400.0), "inside the slack");
    assert!(!zone.keeps(311.0, 400.0), "past the slack");
}
