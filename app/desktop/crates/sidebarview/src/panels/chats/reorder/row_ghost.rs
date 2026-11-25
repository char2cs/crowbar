//! The lifted clone of a row that follows the pointer while it is dragged.

use std::rc::Rc;

use gpui::{AnyElement, App, Context, IntoElement, Render, Window};
use ui::{Portal, layer};

pub(crate) type GhostBuild = Rc<dyn Fn(&mut App) -> AnyElement>;

pub(crate) struct RowGhost {
    build: GhostBuild,
}

impl RowGhost {
    pub(crate) fn new(build: GhostBuild) -> Self {
        Self { build }
    }
}

impl Render for RowGhost {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let build = Rc::clone(&self.build);
        Portal::lifted("row-ghost", move |_, cx| {
            ui::Shaped::seen(build(cx), ui::radius::LG)
        })
        .priority(layer::TOOLTIP)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{
        AppContext, InteractiveElement, StatefulInteractiveElement, Styled, TestAppContext, div, px,
    };

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn the_ghost_draws_whatever_the_row_builds(cx: &mut TestAppContext) {
        let build: GhostBuild = Rc::new(|_| div().h(px(7.0)).into_any_element());
        let bounds = measure(cx, move |_, cx| {
            let build = Rc::clone(&build);
            cx.new(|_| RowGhost::new(build))
        });
        assert_eq!(f32::from(bounds.size.height), 7.0);
    }

    #[derive(Clone)]
    struct Lifted;

    struct Source;

    impl Render for Source {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div().id("row").size(px(40.0)).on_drag(Lifted, lift)
        }
    }

    fn lift(
        _: &Lifted,
        _: gpui::Point<gpui::Pixels>,
        _: &mut Window,
        cx: &mut App,
    ) -> gpui::Entity<RowGhost> {
        cx.new(|_| RowGhost::new(Rc::new(ghost)))
    }

    fn ghost(_: &mut App) -> AnyElement {
        div()
            .h(px(7.0))
            .w(px(30.0))
            .debug_selector(|| "row-ghost".into())
            .into_any_element()
    }

    #[gpui::test]
    fn the_ghost_follows_the_pointer_above_the_panes(cx: &mut TestAppContext) {
        cx.update(|cx| {
            ui::init(cx).expect("ui installs");
            ui::testing::open_float_windows(cx);
        });
        let owner: gpui::AnyWindowHandle = cx.add_window(|_, _| Source).into();
        let mut owner = gpui::VisualTestContext::from_window(owner, cx);
        let modifiers = gpui::Modifiers::none();
        owner.simulate_mouse_down(
            gpui::point(px(5.0), px(5.0)),
            gpui::MouseButton::Left,
            modifiers,
        );
        owner.simulate_mouse_move(
            gpui::point(px(80.0), px(60.0)),
            gpui::MouseButton::Left,
            modifiers,
        );
        owner.run_until_parked();
        assert_eq!(
            owner.debug_bounds("row-ghost"),
            None,
            "nothing in the window"
        );
        let mut top = ui::testing::newest_window(&mut owner);
        assert!(top.debug_bounds("row-ghost").is_some());
    }
}
