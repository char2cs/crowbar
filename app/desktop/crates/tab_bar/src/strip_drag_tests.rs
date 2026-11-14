use model::{Command, TabId};

use super::*;

fn strip(count: u64, width: f32) -> (Vec<TabId>, Vec<Span>) {
    let tabs = (0..count).map(TabId).collect();
    let spans = (0..count)
        .map(|index| Span {
            x: f32::from(u16::try_from(index).expect("small")) * width,
            width,
        })
        .collect();
    (tabs, spans)
}

fn drag(dragged: usize, x: f32) -> StripDrag {
    let (tabs, spans) = strip(4, 100.0);
    StripDrag::begin(tabs, spans, dragged, x).expect("a drag")
}

#[test]
fn pressing_a_tab_does_not_move_it() {
    let drag = drag(1, 150.0);
    assert_eq!(drag.target(), 1, "stays");
    assert_eq!(drag.drop(), None, "dropping in place writes nothing");
    assert_eq!(drag.ghost_x(150.0), 100.0, "ghost sits on the slot");
}

#[test]
fn the_ghost_keeps_its_grab_offset_and_stays_over_the_tabs() {
    let drag = drag(1, 130.0);
    assert_eq!(drag.ghost_x(180.0), 150.0, "follows the pointer");
    assert_eq!(drag.ghost_x(-500.0), 0.0, "clamped at the first slot");
    assert_eq!(
        drag.ghost_x(900.0),
        300.0,
        "clamped so it ends at the last slot"
    );
}

#[test]
fn passing_a_neighbours_middle_by_six_pixels_swaps_places() {
    let mut drag = drag(1, 150.0);
    drag.moved(150.0 + 50.0 + 5.0);
    assert_eq!(drag.target(), 1, "55px is inside the hysteresis");
    drag.moved(150.0 + 57.0);
    assert_eq!(drag.target(), 2, "57px is past the middle plus 6px");
}

#[test]
fn a_long_move_crosses_several_tabs_and_comes_back() {
    let mut drag = drag(0, 50.0);
    drag.moved(390.0);
    assert_eq!(drag.target(), 3, "all the way right");
    drag.moved(10.0);
    assert_eq!(drag.target(), 0, "all the way back");
    assert_eq!(drag.drop(), None, "back where it started");
}

#[test]
fn neighbours_slide_by_the_dragged_width_toward_the_gap() {
    let mut drag = drag(1, 150.0);
    drag.moved(350.0);
    assert_eq!(drag.target(), 3, "moved right two");
    let offsets: Vec<f32> = (0..4).map(|index| drag.offset(index)).collect();
    assert_eq!(
        offsets,
        vec![0.0, 0.0, -100.0, -100.0],
        "tabs 2 and 3 move left"
    );
    let mut left = drag_of(3, 350.0);
    left.moved(120.0);
    let offsets: Vec<f32> = (0..4).map(|index| left.offset(index)).collect();
    assert_eq!(
        offsets,
        vec![0.0, 100.0, 100.0, 0.0],
        "tabs 1 and 2 move right"
    );
}

fn drag_of(dragged: usize, x: f32) -> StripDrag {
    drag(dragged, x)
}

#[test]
fn dropping_on_a_new_slot_moves_the_tab_there() {
    let mut drag = drag(0, 50.0);
    drag.moved(250.0);
    assert_eq!(
        drag.drop(),
        Some(Command::MoveTab {
            tab: TabId(0),
            to_index: 2
        }),
        "move command"
    );
    assert_eq!(drag.tab(), Some(TabId(0)), "the dragged tab");
}

#[test]
fn begin_refuses_mismatched_input() {
    let (tabs, spans) = strip(2, 100.0);
    assert!(
        StripDrag::begin(tabs.clone(), spans.clone(), 5, 0.0).is_none(),
        "index out of range"
    );
    assert!(
        StripDrag::begin(tabs, spans[..1].to_vec(), 0, 0.0).is_none(),
        "length mismatch"
    );
}

#[test]
fn the_ghost_is_pinned_to_the_row_near_it_and_free_far_from_it() {
    let drag = drag(0, 10.0);
    assert_eq!(drag.ghost_y(26.0, 5.0, 21.0), 5.0, "level with the row");
    let eased = drag.ghost_y(26.0 + 16.0, 5.0, 21.0);
    assert!(eased > 5.0 && eased < 21.0, "halfway eased: {eased}");
    assert_eq!(
        drag.ghost_y(26.0 + 200.0, 5.0, 21.0),
        205.0,
        "free beyond the margin"
    );
}
