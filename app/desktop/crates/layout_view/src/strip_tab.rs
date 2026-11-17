use gpui::{
    App, AppContext as _, CursorStyle, ElementId, FontWeight, InteractiveElement, IntoElement,
    MouseButton, MouseDownEvent, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use model::DragSubject;
use protocol::PaneId;
use std::rc::Rc;

use ui::{ContextMenu, DragGhost, Icon, IconName, MenuItem, Palette, Surface, Theme};

use crate::close_button::CloseButton;
use crate::tab_actions::{TabAction, TabActions};
use crate::tab_dragging::TabDragging;
use crate::tab_end::TabEnd;
use crate::tab_state::TabState;

pub(crate) const HEIGHT: f32 = 32.0;
const MAX_WIDTH: f32 = 200.0;
const PAD_LEFT: f32 = 10.0;
const PAD_RIGHT: f32 = 32.0;
const GAP: f32 = 6.0;
const ICON: f32 = 14.0;
const LABEL_SIZE: f32 = 13.0;
const GHOST_WIDTH: f32 = 160.0;
const DROP_LINE: f32 = 2.0;

#[derive(IntoElement)]
pub(crate) struct StripTab {
    pub(crate) pane: PaneId,
    pub(crate) label: SharedString,
    pub(crate) icon: IconName,
    pub(crate) state: TabState,
    pub(crate) end: TabEnd,
    pub(crate) menu: Rc<[MenuItem]>,
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
        let name = self.pane.as_str().to_owned();
        let TabActions {
            select,
            close,
            remove,
            drop,
        } = self.actions;
        let dragging = TabDragging {
            subject: DragSubject::Pane(self.pane),
            ghost: self.label.clone(),
            line: palette.primary(),
            drop,
        };
        let tab = frame(&name, self.state, palette)
            .font_family(theme.ui_family())
            .on_mouse_down(MouseButton::Left, press(select))
            .on_mouse_down(MouseButton::Middle, press(remove));
        let ink = self.state.ink(palette);
        let tab = draggable(tab, dragging)
            .child(Icon::new(self.icon).size(px(ICON)).color(ink))
            .child(label(self.label))
            .child(CloseButton {
                name: name.clone(),
                state: self.state,
                end: self.end,
                action: close,
            });
        div()
            .flex_none()
            .child(ContextMenu::new(group_of(&name), tab, self.menu))
    }
}

fn press(action: TabAction) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) {
    move |_, window, cx| {
        cx.stop_propagation();
        action(window, cx);
    }
}

fn label(text: SharedString) -> gpui::Div {
    div()
        .min_w_0()
        .overflow_hidden()
        .text_ellipsis()
        .child(text)
}

fn draggable(
    tab: gpui::Stateful<gpui::Div>,
    dragging: TabDragging,
) -> gpui::Stateful<gpui::Div> {
    let TabDragging {
        subject,
        ghost,
        line,
        drop,
    } = dragging;
    tab.on_drag(subject, move |_, _, _, cx| {
        cx.new(|_| DragGhost::new(ghost.clone(), GHOST_WIDTH))
    })
    .drag_over::<DragSubject>(move |tab, _, _, _| tab.border_l(px(DROP_LINE)).border_color(line))
    .on_drop::<DragSubject>(move |subject, window, cx| drop(subject, window, cx))
}

pub(crate) fn group_of(name: &str) -> SharedString {
    SharedString::from(format!("strip-tab-{name}"))
}

fn frame(
    name: &str,
    state: TabState,
    palette: &Palette,
) -> gpui::Stateful<gpui::Div> {
    let (fill, ink) = (state.fill(palette), state.ink(palette));
    let (hover_fill, hover_ink) = (palette.sidebar_element_hover(), palette.foreground());
    div()
        .id(ElementId::from(SharedString::from(format!("strip-{name}"))))
        .group(group_of(name))
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
            end: TabEnd::Close,
            menu: Rc::from(Vec::new()),
            actions,
        }
    }

    fn counting(selects: Rc<Cell<u32>>) -> TabActions {
        let select: TabAction = Rc::new(move |_, _| selects.set(selects.get() + 1));
        let nothing: TabAction = Rc::new(|_, _| {});
        let drop: TabDrop = Rc::new(|_, _, _| {});
        TabActions {
            select,
            close: Rc::clone(&nothing),
            remove: nothing,
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
