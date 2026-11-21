use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, CursorStyle, Div, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton,
    MouseDownEvent, ParentElement, RenderOnce, SharedString, Stateful, Styled, Window, div, px,
};
use protocol::PaneId;
use std::rc::Rc;

use ui::{ContextMenu, Icon, MenuItem, Palette, Placement, Surface, Theme, Tooltip, TooltipSide};

use crate::tabbar::close_button::CloseButton;
use crate::tabbar::marks::Marks;
use crate::tabbar::tab_actions::{TabAction, TabActions};
use crate::tabbar::tab_end::TabEnd;
use crate::tabbar::tab_frame::{HEIGHT, group_of};
use crate::tabbar::tab_keep::TabKeep;
use crate::tabbar::tab_look::TabLook;
use crate::tabbar::tab_state::TabState;

const MAX_WIDTH: f32 = 200.0;
const PAD_LEFT: f32 = 10.0;
const PAD_RIGHT: f32 = 32.0;
const GAP: f32 = 6.0;
const ICON: f32 = 14.0;
const LABEL_SIZE: f32 = 13.0;
const DOT: f32 = 8.0;

/// One editor tab of the tab bar.
#[derive(IntoElement)]
pub struct StripTab {
    /// The pane the tab shows.
    pub pane: PaneId,
    /// Its label, glyph and marks.
    pub look: TabLook,
    /// Whether it is the shown tab.
    pub state: TabState,
    /// What its end button does.
    pub end: TabEnd,
    /// Its context menu.
    pub menu: Rc<[MenuItem]>,
    /// What pressing and closing it do.
    pub actions: TabActions,
    /// Whether the keyboard focus ring is drawn on it; the bar sets it.
    pub ringed: bool,
}

impl RenderOnce for StripTab {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        self.element(cx, |tab| tab)
    }
}

impl StripTab {
    pub(crate) fn element(
        self,
        cx: &App,
        carry: impl FnOnce(Stateful<Div>) -> Stateful<Div>,
    ) -> Div {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let name = self.pane.as_str().to_owned();
        let TabActions {
            select,
            close,
            remove,
            keep,
        } = self.actions;
        let (label, icon, dirty) = (self.look.label.clone(), self.look.icon, self.look.dirty);
        let marks = Marks {
            unsaved: dirty && self.state == TabState::Active,
            ringed: self.ringed,
        };
        let tab = framed(&name, self.state, marks, &theme)
            .on_mouse_down(MouseButton::Left, press(select, keep))
            .on_mouse_down(MouseButton::Middle, press(remove, None));
        let ink = self.state.ink(palette);
        let tab = carry(tab)
            .child(Icon::new(icon).size(px(ICON)).color(ink))
            .child(label_of(&self.look))
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

fn framed(
    name: &str,
    state: TabState,
    marks: Marks,
    theme: &Theme,
) -> Stateful<Div> {
    let palette = theme.palette(Surface::Window);
    frame(name, state, palette)
        .when(marks.unsaved, |tab| tab.border_color(palette.primary()))
        .when(marks.ringed, |tab| tab.shadow(ui::focus_ring(palette)))
        .font_family(theme.ui_family())
}

fn press(
    action: TabAction,
    keep: Option<TabAction>,
) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) {
    let keep: TabAction = keep.unwrap_or_else(|| Rc::new(|_, _| {}));
    move |event, window, cx| {
        cx.stop_propagation();
        action(window, cx);
        let _kept = (event.click_count >= 2).then(|| keep(window, cx));
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

fn unsaved_dot(palette: &Palette) -> Div {
    div()
        .flex_none()
        .size(px(DOT))
        .rounded_full()
        .bg(palette.primary())
        .debug_selector(|| "strip-tab-dot".into())
}

fn label_of(look: &TabLook) -> Div {
    div()
        .min_w_0()
        .overflow_hidden()
        .text_ellipsis()
        .when(look.keep == TabKeep::Preview, Styled::italic)
        .child(look.label.clone())
}

fn frame(
    name: &str,
    state: TabState,
    palette: &Palette,
) -> Stateful<Div> {
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

ui::opaque_debug!(StripTab);

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
        TabActions {
            select,
            close: Rc::clone(&nothing),
            remove: nothing,
            keep: None,
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

    struct Kept {
        keeps: Rc<Cell<u32>>,
    }

    impl Render for Kept {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let keeps = Rc::clone(&self.keeps);
            let keep: TabAction = Rc::new(move |_, _| keeps.set(keeps.get() + 1));
            let actions = TabActions {
                keep: Some(keep),
                ..counting(Rc::new(Cell::new(0)))
            };
            div().size_full().child(tab_with(actions))
        }
    }

    fn click(
        cx: &mut gpui::VisualTestContext,
        click_count: usize,
    ) {
        let position = point(px(20.0), px(HEIGHT / 2.0));
        let down = MouseDownEvent {
            button: MouseButton::Left,
            position,
            modifiers: Modifiers::none(),
            click_count,
            first_mouse: false,
        };
        let up = gpui::MouseUpEvent {
            button: MouseButton::Left,
            position,
            modifiers: Modifiers::none(),
            click_count,
        };
        cx.simulate_event(down);
        cx.simulate_event(up);
    }

    #[gpui::test]
    fn a_double_click_keeps_a_preview_tab_and_a_single_one_does_not(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let keeps = Rc::new(Cell::new(0));
        let host = Rc::clone(&keeps);
        let (_, cx) = cx.add_window_view(move |_, _| Kept { keeps: host });
        click(cx, 1);
        click(cx, 2);
        assert_eq!(keeps.get(), 1, "kept once, on the second click");
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
