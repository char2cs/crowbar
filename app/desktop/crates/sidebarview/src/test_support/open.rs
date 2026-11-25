//! Opens a sidebar over a recording data source in a 1200x800 window, with or without the
//! window's top layer.

use std::rc::Rc;

use gpui::{AnyWindowHandle, AppContext as _, Entity, TestAppContext, VisualTestContext, px, size};

use super::{Recorder, blank::Blank};
use sidebar::{DemoShell, Preferences};

use crate::sidebar::Sidebar;

pub(crate) fn open(
    cx: &mut TestAppContext,
    data: DemoShell,
) -> (Rc<Recorder>, Entity<Sidebar>, &mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).expect("ui installs"));
    let recorder = Rc::new(Recorder::new(data));
    let shared = Rc::clone(&recorder);
    let (sidebar, cx) = cx.add_window_view(move |_, cx| {
        let content = cx.new(|_| Blank::default()).into();
        Sidebar::new(
            Rc::clone(&shared) as _,
            shared,
            content,
            (Preferences::default(), cx),
        )
    });
    cx.simulate_resize(size(px(1200.0), px(800.0)));
    cx.run_until_parked();
    (recorder, sidebar, cx)
}

/// The sidebar, its window and the window its menus and dialogs float in: the same window
/// until floating elements get windows of their own.
pub(crate) struct Floating {
    pub(crate) recorder: Rc<Recorder>,
    pub(crate) sidebar: Entity<Sidebar>,
    pub(crate) window: VisualTestContext,
    pub(crate) top: VisualTestContext,
}

pub(crate) fn open_floating(
    cx: &mut TestAppContext,
    data: DemoShell,
) -> Floating {
    cx.update(|cx| ui::init(cx).expect("ui installs"));
    let recorder = Rc::new(Recorder::new(data));
    let shared = Rc::clone(&recorder);
    let handle = cx.add_window(move |_, cx| {
        let content = cx.new(|_| Blank::default()).into();
        Sidebar::new(
            Rc::clone(&shared) as _,
            shared,
            content,
            (Preferences::default(), cx),
        )
    });
    let sidebar = handle.entity(cx).expect("the window shows the sidebar");
    let owner: AnyWindowHandle = handle.into();
    let window = VisualTestContext::from_window(owner, cx);
    window.simulate_resize(size(px(1200.0), px(800.0)));
    cx.run_until_parked();
    Floating {
        recorder,
        sidebar,
        window,
        top: VisualTestContext::from_window(owner, cx),
    }
}

/// Presses and releases each space-separated key, as a keyboard does; buttons click on release.
pub(crate) fn press(
    cx: &mut VisualTestContext,
    keys: &'static str,
) {
    for key in keys.split(' ') {
        cx.simulate_keystrokes(key);
        let keystroke = gpui::Keystroke::parse(key).expect("a valid keystroke");
        cx.simulate_event(gpui::KeyUpEvent { keystroke });
        cx.run_until_parked();
    }
}

/// Lets a closed floating element go: the test platform draws inside an effect
/// flush, so the state of a portal that stopped rendering is released on the next one.
pub(crate) fn settle(cx: &mut VisualTestContext) {
    cx.update(|_, _| {});
    cx.run_until_parked();
}
