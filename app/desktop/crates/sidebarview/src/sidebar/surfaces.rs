use gpui::{Context, Window};
use sidebar::{Mode, Visibility};
use ui::Motion;

use crate::sidebar::Sidebar;
use crate::sidebar::layout::geometry::Geometry;

impl Sidebar {
    pub fn is_drawer_open(
        &self,
        window: &Window,
    ) -> bool {
        self.geometry(window).is_drawer()
    }

    pub fn shows_files(
        &self,
        window: &Window,
    ) -> bool {
        let geometry = self.geometry(window);
        self.mode == Mode::Files && (geometry.is_docked() || geometry.is_drawer())
    }

    pub fn dismiss_drawer(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if !self.is_open() {
            return;
        }
        self.peek.slide_to(0.0, Motion::of(cx));
        self.panel.toggle();
        self.changed(cx);
    }

    pub fn search_files(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match self.search.as_ref() {
            Some(search) => search.field.update(cx, |field, cx| field.focus(window, cx)),
            None => self.open_file_search(window, cx),
        }
    }

    fn is_open(&self) -> bool {
        self.panel.visibility() == Visibility::Open
    }

    fn geometry(
        &self,
        window: &Window,
    ) -> Geometry {
        let container = Self::container_width(window);
        Geometry::new(container, self.live_width(container), self.panel)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{px, size};

    use super::*;
    use crate::test_support::prelude::*;

    type Case = (&'static str, f32, Mode, (bool, bool));

    const CASES: &[Case] = &[
        ("docked chats", 1200.0, Mode::Chats, (false, false)),
        ("docked files", 1200.0, Mode::Files, (false, true)),
        ("files in a drawer", 600.0, Mode::Files, (true, true)),
        ("chats in a drawer", 600.0, Mode::Chats, (true, false)),
    ];

    fn shown(
        sidebar: &gpui::Entity<Sidebar>,
        cx: &mut gpui::VisualTestContext,
    ) -> (bool, bool) {
        sidebar.update_in(cx, |this, window, _| {
            (this.is_drawer_open(window), this.shows_files(window))
        })
    }

    #[gpui::test]
    fn the_drawer_and_the_files_tree_show_by_window_width_and_mode(cx: &mut TestAppContext) {
        for (name, width, mode, wanted) in CASES {
            let (_, sidebar, cx) = open(&mut *cx, DemoShell::reference());
            cx.simulate_resize(size(px(*width), px(800.0)));
            sidebar.update(cx, |this, cx| this.set_mode(*mode, cx));
            assert_eq!(shown(&sidebar, cx), *wanted, "{name}");
        }
    }

    #[gpui::test]
    fn escape_on_the_drawer_closes_it_and_nothing_shows(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        cx.simulate_resize(size(px(600.0), px(800.0)));
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        sidebar.update(cx, Sidebar::dismiss_drawer);
        assert_eq!(shown(&sidebar, cx), (false, false));
    }
}
