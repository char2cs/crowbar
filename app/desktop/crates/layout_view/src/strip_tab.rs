use gpui::{
    App, AppContext as _, CursorStyle, ElementId, FontWeight, Hsla, InteractiveElement,
    IntoElement, MouseButton, ParentElement, RenderOnce, SharedString, StatefulInteractiveElement,
    Styled, Window, div, px,
};
use model::DragSubject;
use protocol::PaneId;
use ui::{DragGhost, Icon, IconName, Palette, Surface, Theme};

use crate::tab_actions::{TabAction, TabActions};
use crate::tab_state::TabState;

pub(crate) const HEIGHT: f32 = 32.0;
const MAX_WIDTH: f32 = 200.0;
const PAD_LEFT: f32 = 10.0;
const PAD_RIGHT: f32 = 32.0;
const GAP: f32 = 6.0;
const ICON: f32 = 14.0;
const CLOSE: f32 = 20.0;
const CLOSE_INSET: f32 = 6.0;
const CLOSE_GLYPH: f32 = 12.0;
const CLOSE_OPACITY: f32 = 0.6;
const LABEL_SIZE: f32 = 13.0;
const GHOST_WIDTH: f32 = 160.0;
const DROP_LINE: f32 = 2.0;

#[derive(IntoElement)]
pub(crate) struct StripTab {
    pub(crate) pane: PaneId,
    pub(crate) label: SharedString,
    pub(crate) icon: IconName,
    pub(crate) state: TabState,
    pub(crate) actions: TabActions,
}

impl RenderOnce for StripTab {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let ink = self.state.ink(palette);
        let name = self.pane.as_str().to_owned();
        let group = SharedString::from(format!("strip-tab-{name}"));
        let (fill, rest) = (self.state.fill(palette), self.state.rest_opacity());
        let TabActions {
            select,
            close,
            drop,
        } = self.actions;
        let ghost = self.label.clone();
        let subject = DragSubject::Pane(self.pane);
        let line = palette.primary();
        frame(&name, &group, (fill, ink, palette))
            .font_family(theme.ui_family())
            .on_mouse_down(MouseButton::Left, move |_, window, cx| select(window, cx))
            .on_drag(subject, move |_, _, _, cx| {
                cx.new(|_| DragGhost::new(ghost.clone(), GHOST_WIDTH))
            })
            .drag_over::<DragSubject>(move |tab, _, _, _| {
                tab.border_l(px(DROP_LINE)).border_color(line)
            })
            .on_drop::<DragSubject>(move |subject, window, cx| drop(subject, window, cx))
            .child(Icon::new(self.icon).size(px(ICON)).color(ink))
            .child(
                div()
                    .min_w_0()
                    .overflow_hidden()
                    .text_ellipsis()
                    .child(self.label),
            )
            .child(close_button((&name, &group), (rest, ink), close))
    }
}

fn frame(
    name: &str,
    group: &SharedString,
    (fill, ink, palette): (Hsla, Hsla, &Palette),
) -> gpui::Stateful<gpui::Div> {
    let (hover_fill, hover_ink) = (palette.sidebar_element_hover(), palette.foreground());
    div()
        .id(ElementId::from(SharedString::from(format!("strip-{name}"))))
        .group(group.clone())
        .relative()
        .flex()
        .items_center()
        .gap(px(GAP))
        .h(px(HEIGHT))
        .max_w(px(MAX_WIDTH))
        .pl(px(PAD_LEFT))
        .pr(px(PAD_RIGHT))
        .rounded(ui::radius::SM)
        .bg(fill)
        .text_color(ink)
        .text_size(px(LABEL_SIZE))
        .font_weight(FontWeight::MEDIUM)
        .whitespace_nowrap()
        .cursor(CursorStyle::PointingHand)
        .hover(move |tab| tab.bg(hover_fill).text_color(hover_ink))
}

fn close_button(
    (name, group): (&str, &SharedString),
    (rest, ink): (f32, Hsla),
    on_close: TabAction,
) -> impl IntoElement {
    div()
        .id(ElementId::from(SharedString::from(format!(
            "strip-close-{name}"
        ))))
        .absolute()
        .right(px(CLOSE_INSET))
        .top(px((HEIGHT - CLOSE) / 2.0))
        .size(px(CLOSE))
        .flex()
        .items_center()
        .justify_center()
        .rounded(ui::radius::MD)
        .opacity(rest)
        .group_hover(group.clone(), |button| button.opacity(CLOSE_OPACITY))
        .on_mouse_down(MouseButton::Left, move |_, window, cx| {
            cx.stop_propagation();
            on_close(window, cx);
        })
        .child(Icon::new(IconName::X).size(px(CLOSE_GLYPH)).color(ink))
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{
        Context, IntoElement, Modifiers, MouseButton, Render, TestAppContext, Window, div, point,
        px,
    };
    use ui::{IconName, Motion, Theme};

    use super::*;
    use crate::tab_actions::TabDrop;

    fn tab_with(actions: TabActions) -> StripTab {
        StripTab {
            pane: PaneId::new("a").expect("valid"),
            label: "File".into(),
            icon: IconName::X,
            state: TabState::Idle,
            actions,
        }
    }

    fn counting(selects: Rc<Cell<u32>>) -> TabActions {
        let select: TabAction = Rc::new(move |_, _| selects.set(selects.get() + 1));
        let nothing: TabAction = Rc::new(|_, _| {});
        let drop: TabDrop = Rc::new(|_, _, _| {});
        TabActions {
            select,
            close: nothing,
            drop,
        }
    }

    struct Host {
        selects: Rc<Cell<u32>>,
    }

    impl Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let actions = counting(Rc::clone(&self.selects));
            div().size_full().child(tab_with(actions))
        }
    }

    #[gpui::test]
    fn pressing_a_strip_tab_selects_it(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let selects = Rc::new(Cell::new(0));
        let host = Rc::clone(&selects);
        let (_, cx) = cx.add_window_view(move |_, _| Host { selects: host });
        let at = point(px(20.0), px(HEIGHT / 2.0));
        cx.simulate_mouse_down(at, MouseButton::Left, Modifiers::none());
        assert_eq!(selects.get(), 1, "one select");
    }
}
