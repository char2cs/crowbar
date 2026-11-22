use gpui::{Context, Entity, IntoElement, ParentElement, Render, Window, div};
use ui::TabPreview;

use crate::tabbar::strip_drag::StripDrag;
use crate::tabbar::strip_state::StripState;

pub(crate) struct TabGhost {
    pub(crate) pill: Entity<TabPreview>,
    pub(crate) strip: Entity<StripState>,
}

impl Render for TabGhost {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let lifted = self
            .strip
            .read(cx)
            .drag
            .as_ref()
            .is_some_and(StripDrag::armed);
        div().children(lifted.then(|| self.pill.clone()))
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};
    use ui::{Motion, Theme};

    use super::*;
    use crate::test_support::dragged;

    #[gpui::test]
    fn the_pill_shows_only_once_the_tab_is_lifted(cx: &mut TestAppContext) {
        cx.update(|cx| {
            ui::init(cx).expect("ui init");
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let strip = cx.update(|cx| cx.new(StripState::new));
        let held = strip.clone();
        let (_, cx) = cx.add_window_view(move |_, cx| TabGhost {
            pill: cx.new(|_| TabPreview::new("main.rs")),
            strip: held,
        });
        let mut shown = Vec::new();
        for moves in [&[4.0][..], &[8.0][..]] {
            strip.update(cx, |state, _| state.drag = Some(dragged(0, moves)));
            cx.update(|window, _| window.refresh());
            cx.run_until_parked();
            shown.push(cx.debug_bounds("drag-ghost").is_some());
        }
        assert_eq!(shown, [false, true], "hidden at 4 px, shown at 8 px");
    }
}
