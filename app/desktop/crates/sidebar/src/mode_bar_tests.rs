use super::*;
use crate::test_support::prelude::*;

use crate::focus::Focus;

#[gpui::test]
fn choosing_a_mode_in_a_repository_shows_it(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
    cx.run_until_parked();
    assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Changes);
}

#[gpui::test]
fn a_mode_the_focus_cannot_use_is_refused(cx: &mut TestAppContext) {
    let data = DemoData::reference().with_focus(Focus::Nothing);
    let (_, sidebar, cx) = open(cx, data);
    sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
    assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Chats);
}

#[test]
fn the_active_tab_alone_keeps_its_label_in_a_narrow_column() {
    let focus = Focus::Repository;
    let active = item(
        Mode::Files,
        LabelVisibility::ActiveOnly,
        (Mode::Files, focus),
    );
    let idle = item(
        Mode::Chats,
        LabelVisibility::ActiveOnly,
        (Mode::Files, focus),
    );
    assert_ne!(format!("{active:?}"), format!("{idle:?}"));
}
