use std::cell::Cell;
use std::rc::Rc;

use gpui::{ParentElement, Styled, TestAppContext, point, px};

use super::*;
use crate::icon_name::IconName;
use crate::test_support::{measure, open};

fn items() -> Vec<TabItem> {
    vec![TabItem::icon(IconName::ChatsCircle), TabItem::icon(IconName::FolderOpen), TabItem::icon(IconName::GitBranch)]
}

#[gpui::test]
fn list_is_thirty_six_pixels_high_in_pill_style_and_thirty_four_segmented(cx: &mut TestAppContext) {
    let pill = measure(cx, |_, _| gpui::div().w(px(300.0)).child(Tabs::new("t", items(), 0)));
    assert_eq!(f32::from(pill.size.height), 36.0);
    let segmented = measure(cx, |_, _| gpui::div().w(px(300.0)).child(Tabs::new("t", items(), 0).style(TabsStyle::Segmented)));
    assert_eq!(f32::from(segmented.size.height), 34.0);
}

#[gpui::test]
fn clicking_an_unselected_tab_reports_its_index(cx: &mut TestAppContext) {
    let seen = Rc::new(Cell::new(None));
    let sink = Rc::clone(&seen);
    let cx = open(cx, move |_, _| {
        let sink = Rc::clone(&sink);
        gpui::div().w(px(300.0)).child(Tabs::new("t", items(), 0).on_select(move |index, _, _| sink.set(Some(index))))
    });
    cx.simulate_click(point(px(150.0), px(18.0)), gpui::Modifiers::none());
    assert_eq!(seen.get(), Some(1));
    seen.set(None);
    cx.simulate_click(point(px(20.0), px(18.0)), gpui::Modifiers::none());
    assert_eq!(seen.get(), None, "the selected tab does not re-select");
}
