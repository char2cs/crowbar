use super::*;
use crate::test_support::prelude::*;

#[gpui::test]
fn the_column_follows_the_shown_mode(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    for mode in [Mode::Files, Mode::Changes, Mode::Chats] {
        sidebar.update(cx, |this, cx| this.set_mode(mode, cx));
        cx.run_until_parked();
        assert_eq!(sidebar.read_with(cx, |this, _| this.mode), mode);
    }
}
