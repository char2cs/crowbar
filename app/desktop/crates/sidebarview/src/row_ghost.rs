//! The lifted clone of a row that follows the pointer while it is dragged.

use std::rc::Rc;

use gpui::{AnyElement, App, Context, IntoElement, Render, Window};

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
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        (self.build)(cx)
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext, Styled, TestAppContext, div, px};

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
}
