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

/// The sidebar and its window, whose menus and dialogs float in windows of their own.
pub(crate) struct Floating {
    pub(crate) recorder: Rc<Recorder>,
    pub(crate) sidebar: Entity<Sidebar>,
    pub(crate) window: VisualTestContext,
}

impl Floating {
    /// The float window the owner opened last; the owner's own window while it has none.
    pub(crate) fn top(&mut self) -> VisualTestContext {
        ui::testing::newest_window(&mut self.window)
    }
}

pub(crate) fn open_floating(
    cx: &mut TestAppContext,
    data: DemoShell,
) -> Floating {
    cx.update(|cx| {
        ui::init(cx).expect("ui installs");
        ui::testing::open_float_windows(cx);
    });
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
    }
}

/// Presses and releases each space-separated key, as a keyboard does; buttons click on release.
/// A key that closes the window, as escape closes a float window, ends the presses.
pub(crate) fn press(
    cx: &mut VisualTestContext,
    keys: &'static str,
) {
    let window = cx.update(|window, _| window.window_handle());
    keys.split(' ')
        .map_while(|key| press_one(cx, window, key))
        .for_each(drop);
}

fn press_one(
    cx: &mut VisualTestContext,
    window: AnyWindowHandle,
    key: &'static str,
) -> Option<()> {
    cx.simulate_keystrokes(key);
    cx.windows().contains(&window).then_some(())?;
    let keystroke = gpui::Keystroke::parse(key).expect("a valid keystroke");
    cx.simulate_event(gpui::KeyUpEvent { keystroke });
    cx.run_until_parked();
    Some(())
}

/// Lets a closed floating element go: the test platform draws inside an effect
/// flush, so the state of a portal that stopped rendering is released on the next one.
pub(crate) fn settle(cx: &mut VisualTestContext) {
    cx.update(|_, _| {});
    cx.run_until_parked();
}
