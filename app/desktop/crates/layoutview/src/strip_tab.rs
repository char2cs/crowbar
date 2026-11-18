use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, AppContext as _, CursorStyle, ElementId, FontWeight, InteractiveElement, IntoElement,
    MouseButton, MouseDownEvent, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use layout::DragSubject;
use protocol::PaneId;
use std::rc::Rc;

use ui::{
    ContextMenu, DragGhost, Icon, MenuItem, Palette, Placement, Surface, Theme, Tooltip,
    TooltipSide,
};

use crate::close_button::CloseButton;
use crate::marks::Marks;
use crate::tab_actions::{TabAction, TabActions, TabDrop};
use crate::tab_dragging::TabDragging;
use crate::tab_end::TabEnd;
use crate::tab_frame::{HEIGHT, group_of};
use crate::tab_look::TabLook;
use crate::tab_state::TabState;

const MAX_WIDTH: f32 = 200.0;
const PAD_LEFT: f32 = 10.0;
const PAD_RIGHT: f32 = 32.0;
const GAP: f32 = 6.0;
const ICON: f32 = 14.0;
const LABEL_SIZE: f32 = 13.0;
const GHOST_WIDTH: f32 = 160.0;
const DROP_LINE: f32 = 2.0;
const DOT: f32 = 8.0;

#[derive(IntoElement)]
pub(crate) struct StripTab {
    pub(crate) pane: PaneId,
    pub(crate) look: TabLook,
    pub(crate) state: TabState,
    pub(crate) end: TabEnd,
    pub(crate) menu: Rc<[MenuItem]>,
    pub(crate) actions: TabActions,
    pub(crate) ringed: bool,
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
        let (label, icon, dirty) = (self.look.label.clone(), self.look.icon, self.look.dirty);
        let dragging = dragging_of(self.pane, (label.clone(), drop), palette);
        let marks = Marks {
            unsaved: dirty && self.state == TabState::Active,
            ringed: self.ringed,
        };
        let tab = framed(&name, self.state, marks, &theme)
            .on_mouse_down(MouseButton::Left, press(select))
            .on_mouse_down(MouseButton::Middle, press(remove));
        let ink = self.state.ink(palette);
        let tab = draggable(tab, dragging)
            .child(Icon::new(icon).size(px(ICON)).color(ink))
            .child(label_of(label.clone()))
            .children((dirty && self.state == TabState::Idle).then(|| unsaved_dot(palette)))
            .child(CloseButton {
                name: name.clone(),
                state: self.state,
                end: self.end,
                action: close,
            });
        let tip = tooltip_text(&label, self.look.path.as_deref());
        div()
            .flex_none()
            .child(with_menu(&name, (tip, tab), self.menu))
    }
}

fn dragging_of(
    pane: PaneId,
    (ghost, drop): (SharedString, TabDrop),
    palette: &Palette,
) -> TabDragging {
    TabDragging {
        subject: DragSubject::Pane(pane),
        ghost,
        line: palette.primary(),
        drop,
    }
}

fn framed(
    name: &str,
    state: TabState,
    marks: Marks,
    theme: &Theme,
) -> gpui::Stateful<gpui::Div> {
    let palette = theme.palette(Surface::Window);
    frame(name, state, palette)
        .when(marks.unsaved, |tab| tab.border_color(palette.primary()))
        .when(marks.ringed, |tab| tab.shadow(ui::focus_ring(palette)))
        .font_family(theme.ui_family())
}

fn press(action: TabAction) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) {
    move |_, window, cx| {
        cx.stop_propagation();
        action(window, cx);
    }
}

fn frame_selector(name: &str) -> impl FnOnce() -> String + use<> {
    let name = name.to_owned();
    move || format!("strip-tab-frame-{name}")
}

fn tip_id(name: &str) -> ElementId {
    ElementId::from(SharedString::from(format!("strip-tip-{name}")))
}

fn tooltip_text(
    label: &str,
    path: Option<&str>,
) -> SharedString {
    match path {
        Some(path) => format!("{label}\n{path}").into(),
        None => label.to_owned().into(),
    }
}

fn with_menu(
    name: &str,
    (tip, tab): (SharedString, impl IntoElement),
    menu: Rc<[MenuItem]>,
) -> ContextMenu {
    let tipped = Tooltip::new(tip_id(name), tip, tab).side(TooltipSide::Bottom);
    ContextMenu::new(group_of(name), tipped, menu).placement(Placement::BelowCentre)
}

fn unsaved_dot(palette: &Palette) -> gpui::Div {
    div()
        .flex_none()
        .size(px(DOT))
        .rounded_full()
        .bg(palette.primary())
        .debug_selector(|| "strip-tab-dot".into())
}

fn label_of(text: SharedString) -> gpui::Div {
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
        .debug_selector(frame_selector(name))
        .relative()
        .flex()
        .items_center()
        .gap(px(GAP))
        .h(px(HEIGHT))
        .max_w(px(MAX_WIDTH))
        .pl(px(PAD_LEFT))
        .pr(px(PAD_RIGHT))
        .rounded(ui::radius::SM)
        .border_1()
        .border_color(gpui::transparent_black())
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
    use ui::{Motion, Theme};

    use super::*;

    fn tab_with(actions: TabActions) -> StripTab {
        StripTab {
            pane: PaneId::new("a").expect("valid"),
            look: TabLook::plain("File"),
            state: TabState::Idle,
            end: TabEnd::Close,
            menu: Rc::from(Vec::new()),
            actions,
            ringed: false,
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

    struct Shown {
        look: TabLook,
        state: TabState,
    }

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let tab = StripTab {
                look: self.look.clone(),
                state: self.state,
                ..tab_with(counting(Rc::new(Cell::new(0))))
            };
            div().size_full().child(tab)
        }
    }

    fn dot_shown(
        cx: &mut TestAppContext,
        (look, state): (TabLook, TabState),
    ) -> bool {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (_, cx) = cx.add_window_view(move |_, _| Shown { look, state });
        cx.run_until_parked();
        cx.debug_bounds("strip-tab-dot").is_some()
    }

    #[gpui::test]
    fn only_an_unshown_tab_with_unsaved_work_carries_the_dot(cx: &mut TestAppContext) {
        let unsaved = TabLook::plain("File").unsaved();
        let saved = TabLook::plain("File");
        assert!(
            dot_shown(cx, (unsaved.clone(), TabState::Idle)),
            "unsaved and hidden"
        );
        assert!(
            !dot_shown(cx, (unsaved, TabState::Active)),
            "the shown one has the border"
        );
        assert!(!dot_shown(cx, (saved, TabState::Idle)), "nothing to save");
    }

    #[test]
    fn the_tooltip_names_the_tab_and_adds_the_file_on_a_second_line() {
        let cases = [
            (
                "a file",
                "main.rs",
                Some("src/main.rs"),
                "main.rs\nsrc/main.rs",
            ),
            ("no file", "Terminal", None, "Terminal"),
        ];
        for (name, label, path, expected) in cases {
            assert_eq!(tooltip_text(label, path).as_ref(), expected, "{name}");
        }
    }
}
