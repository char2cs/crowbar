use crate::sidebar::Sidebar;
use crate::test_support::prelude::*;
use crate::visibility::Visibility;

#[gpui::test]
fn the_toggle_closes_and_reopens_the_sidebar(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    sidebar.update(cx, Sidebar::toggle);
    assert!(sidebar.read_with(cx, |this, _| this.panel().visibility()
        == Visibility::Closed));
    sidebar.update(cx, Sidebar::toggle);
    assert!(sidebar.read_with(cx, |this, _| this.panel().visibility() == Visibility::Open));
}

#[gpui::test]
fn the_current_host_is_the_one_of_the_active_space(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    let name = sidebar.read_with(cx, |this, _| {
        let hosts = this.data.hosts();
        this.current_host(&hosts).map(|host| host.name)
    });
    assert!(name.is_some());
}

#[gpui::test]
fn a_single_host_shows_no_picker(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let picker = sidebar.update(cx, |this, cx| this.host_choice(cx));
    assert!(picker.is_none());
}
