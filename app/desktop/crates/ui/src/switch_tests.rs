use std::cell::Cell;
use std::rc::Rc;

use gpui::{TestAppContext, point, px};

use super::*;
use crate::test_support::{measure, open};

#[gpui::test]
fn track_is_thirty_by_eighteen(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| Switch::new("s", true));
    assert_eq!((f32::from(bounds.size.width), f32::from(bounds.size.height)), (30.0, 18.0));
}

#[gpui::test]
fn click_requests_the_opposite_value(cx: &mut TestAppContext) {
    const CASES: &[(bool, bool)] = &[(true, false), (false, true)];
    for &(checked, expected) in CASES {
        let seen = Rc::new(Cell::new(None));
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            Switch::new("s", checked).on_change(move |value, _, _| sink.set(Some(value)))
        });
        cx.simulate_click(point(px(10.0), px(9.0)), gpui::Modifiers::none());
        assert_eq!(seen.get(), Some(expected), "checked {checked}");
    }
}

#[gpui::test]
fn disabled_switch_ignores_clicks(cx: &mut TestAppContext) {
    let seen = Rc::new(Cell::new(false));
    let sink = Rc::clone(&seen);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        Switch::new("s", true).disabled(true).on_change(move |_, _, _| sink.set(true))
    });
    cx.simulate_click(point(px(10.0), px(9.0)), gpui::Modifiers::none());
    assert!(!seen.get());
}
