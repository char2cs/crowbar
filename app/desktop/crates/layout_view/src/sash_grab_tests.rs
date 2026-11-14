use model::{Command, Edge, Rect};

use super::*;
use crate::test_support::layout_of;

const CONTENT: Rect = Rect {
    x: 100.0,
    y: 40.0,
    width: 806.0,
    height: 600.0,
};

fn grab_at(layout: &model::Layout, pointer: (f32, f32)) -> SashGrab {
    let tab = layout.active_tab().expect("tab");
    let sash = tab.sashes(CONTENT).remove(0);
    SashGrab::begin(tab, &sash, pointer, CONTENT).expect("grab")
}

#[test]
fn moving_the_pointer_by_forty_pixels_grows_the_first_child_by_forty() {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let grab = grab_at(&layout, (503.0, 300.0));
    let Command::ResizeSplit {
        first_extent,
        bounds,
        ..
    } = grab.resize((543.0, 310.0))
    else {
        panic!("a resize command");
    };
    assert_eq!(first_extent, 440.0, "400 plus 40");
    assert_eq!(bounds, CONTENT, "bounds of the tab");
}

#[test]
fn a_vertical_split_follows_the_vertical_pointer_only() {
    let layout = layout_of("a", &[("b", "a", Edge::Bottom)]);
    let grab = grab_at(&layout, (300.0, 340.0));
    let Command::ResizeSplit { first_extent, .. } = grab.resize((900.0, 300.0)) else {
        panic!("a resize command");
    };
    assert_eq!(first_extent, 297.0 - 40.0, "297 minus 40");
}

#[test]
fn the_command_applies_and_moves_the_sash() {
    let mut layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let grab = grab_at(&layout, (503.0, 300.0));
    layout
        .apply(grab.resize((603.0, 300.0)))
        .expect("resize applies");
    let tab = layout.active_tab().expect("tab");
    assert_eq!(
        tab.sashes(CONTENT)[0].rect.x,
        600.0,
        "sash followed the pointer by 100px"
    );
}

#[test]
fn reset_names_the_same_split() {
    let layout = layout_of("a", &[("b", "a", Edge::Right)]);
    let grab = grab_at(&layout, (503.0, 300.0));
    let Command::ResetSplit { path, .. } = grab.reset() else {
        panic!("a reset command");
    };
    assert!(path.is_empty(), "the root split");
}
