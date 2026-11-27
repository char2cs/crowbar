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

pub(crate) struct Floating {
    pub(crate) recorder: Rc<Recorder>,
    pub(crate) sidebar: Entity<Sidebar>,
    pub(crate) window: VisualTestContext,
}

impl Floating {
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
