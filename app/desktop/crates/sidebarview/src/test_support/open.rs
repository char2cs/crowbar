//! Opens a sidebar over a recording data source in a 1200x800 window.

use std::rc::Rc;

use gpui::{AppContext as _, Entity, TestAppContext, VisualTestContext, px, size};

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
