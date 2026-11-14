use std::cell::Cell;
use std::rc::Rc;

use gpui::{Modifiers, TestAppContext, point, px};

use super::*;
use crate::icon_name::IconName;
use crate::surface::Surface;
use crate::test_support::{measure, open};
use crate::theme::Theme;

fn look() -> (TabsStyle, &'static Palette) {
    (TabsStyle::Pill, Theme::default().palette(Surface::Window))
}

fn cell() -> TabCell {
    TabCell::new(
        ElementId::from("tab"),
        TabItem::icon(IconName::Gear),
        look(),
    )
}

#[gpui::test]
fn height_follows_the_style(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| cell());
    assert!((f32::from(bounds.size.height) - 32.0).abs() < f32::EPSILON);
}

#[gpui::test]
fn only_an_enabled_unselected_tab_reports_clicks(cx: &mut TestAppContext) {
    let clicked = Rc::new(Cell::new(0));
    let sink = Rc::clone(&clicked);
    let handler: IndexHandler = Rc::new(move |_, _, _| sink.set(sink.get() + 1));
    let cx = open(cx, move |_, _| {
        cell().at(3, Pose::Off).on_select(Some(Rc::clone(&handler)))
    });
    cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
    assert_eq!(clicked.get(), 1);
}

#[gpui::test]
fn selected_tab_ignores_clicks(cx: &mut TestAppContext) {
    let clicked = Rc::new(Cell::new(0));
    let sink = Rc::clone(&clicked);
    let handler: IndexHandler = Rc::new(move |_, _, _| sink.set(sink.get() + 1));
    let cx = open(cx, move |_, _| {
        cell().at(0, Pose::On).on_select(Some(Rc::clone(&handler)))
    });
    cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
    assert_eq!(clicked.get(), 0);
}
