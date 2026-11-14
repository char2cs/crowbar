use std::time::Instant;

use gpui::{TextRun, hsla, px};

use super::*;

fn base(len: usize) -> TextRun {
    TextRun {
        len,
        font: gpui::font("Cal Sans UI"),
        color: hsla(0.0, 0.0, 0.0, 1.0),
        background_color: None,
        underline: None,
        strikethrough: None,
    }
}

#[test]
fn unmarked_text_is_one_run() {
    let runs = runs_for(5, None, &base(5));
    assert_eq!(runs.iter().map(|run| run.len).collect::<Vec<_>>(), [5]);
}

#[test]
fn marked_text_is_underlined_between_plain_runs() {
    let runs = runs_for(6, Some(2..4), &base(6));
    assert_eq!(
        runs.iter().map(|run| run.len).collect::<Vec<_>>(),
        [2, 2, 2]
    );
    assert!(
        runs[1].underline.is_some() && runs[0].underline.is_none() && runs[2].underline.is_none()
    );
    assert_eq!(
        runs[1].underline.map(|underline| underline.thickness),
        Some(px(1.0))
    );
}

#[test]
fn marks_at_the_edges_drop_empty_runs() {
    let runs = runs_for(4, Some(0..4), &base(4));
    assert_eq!(runs.len(), 1);
}

#[test]
fn caret_is_solid_when_motion_is_reduced_and_visible_at_the_start_of_a_blink() {
    let colour = hsla(0.5, 0.5, 0.5, 1.0);
    assert_eq!(
        caret_color(colour, Instant::now(), Motion::reduced()),
        colour
    );
    assert!(caret_color(colour, Instant::now(), Motion::default()).a > 0.9);
}
