use gpui::{
    App, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, canvas, div, px,
};
use protocol::PaneId;
use ui::{IconName, MenuAction, MenuItem, Palette, Placement, Surface, Theme};

use crate::tabbar::carried_tab::CarriedTab;
use crate::tabbar::chat_tab::ChatTab;
use crate::tabbar::input_mode::InputMode;
use crate::tabbar::overflow_entry::OverflowEntry;
use crate::tabbar::review_button::review_button;
use crate::tabbar::strip_pointer;
use crate::tabbar::strip_state::StripState;
use crate::tabbar::strip_tab::StripTab;
use crate::tabbar::tab_actions::{StripKeys, TabAction, TabMove};
use crate::tabbar::tab_state::TabState;
use crate::toprow::row_button::RowButton;
use crate::toprow::row_ink::RowInk;
use crate::toprow::split_toggle::SplitToggle;

/// The height of the tab bar row.
pub const STRIP_HEIGHT: f32 = 42.0;
const PAD_X: f32 = 8.0;
const PAD_Y: f32 = 4.0;
const GAP: f32 = 6.0;
const MORE_SIZE: f32 = 24.0;
const MORE_GLYPH: f32 = 14.0;

type Shown = (PaneId, usize);

/// The tab bar of one pane group: its editor tabs, the "+" and the overflow menu.
#[derive(IntoElement)]
pub struct PaneStrip {
    /// The pane the bar's kept state is keyed by.
    pub key: PaneId,
    /// The split toggle leading the row.
    pub toggle: SplitToggle,
    /// The chat's entry before the editor tabs, when the group shows one view at a time.
    pub chat_tab: Option<ChatTab>,
    /// The editor tabs, in order; with none, the row holds only the toggle.
    pub tabs: Vec<StripTab>,
    /// The index of the shown tab.
    pub shown: usize,
    /// What releasing a tab dragged along the bar does.
    pub reorder: TabMove,
    /// What the "+" does.
    pub add: TabAction,
    /// What a key pressed on the focused bar does.
    pub keys: StripKeys,
    /// The rows of the overflow menu.
    pub overflow: Vec<OverflowEntry>,
    /// What the "Review this branch" button ending the row does; `None` when the group's chat
    /// has no branch to review, and the row has no such button.
    pub review: Option<TabAction>,
}

impl RenderOnce for PaneStrip {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let name = self.key.as_str();
        let state = window.use_keyed_state(id_of(name, "state"), cx, |_, cx| StripState::new(cx));
        let shown = shown_of(&self);
        let (focus, scroll, overflowing) = {
            let held = state.read(cx);
            (held.focus.clone(), held.scroll.clone(), held.overflowing)
        };
        let mode = StripState::shown_mode(&state, &focus, (window, cx));
        let keys = self.keys;
        let (measuring, typing, pressing) = (state.clone(), state.clone(), state.clone());
        let landing = strip_pointer::probe(state.clone(), self.reorder);
        let tabs = (!self.tabs.is_empty()).then(|| {
            scroller(name, &scroll)
                .children(self.chat_tab)
                .children(CarriedTab::seated(ringing(self.tabs, mode), &state, cx))
                .child(add_button(name, self.add))
        });
        let more = overflowing.then(|| more_tabs(name, self.overflow, palette));
        row(name, palette)
            .track_focus(&focus)
            .on_key_down(move |event, window, cx| {
                StripState::use_mode(&typing, InputMode::Keyboard, cx);
                keys(event.keystroke.key.as_str(), window, cx);
            })
            .capture_any_mouse_down(move |_, window, cx| {
                StripState::use_mode(&pressing, InputMode::Pointer, cx);
                window.focus(&focus, cx);
            })
            .child(self.toggle)
            .children(tabs)
            .child(probe(&scroll, (measuring, shown)))
            .child(landing)
            .children(more)
            .children(self.review.map(|review| review_button(name, review)))
    }
}

fn row(
    name: &str,
    palette: &Palette,
) -> gpui::Stateful<gpui::Div> {
    div()
        .id(id_of(name, "row"))
        .flex()
        .items_center()
        .gap(px(GAP))
        .px(px(PAD_X))
        .py(px(PAD_Y))
        .w_full()
        .h(px(STRIP_HEIGHT))
        .overflow_hidden()
        .bg(row_fill(palette))
}

fn row_fill(palette: &Palette) -> gpui::Hsla {
    palette.pane_background()
}

fn ringing(
    tabs: Vec<StripTab>,
    mode: InputMode,
) -> Vec<StripTab> {
    tabs.into_iter()
        .map(|tab| StripTab {
            ringed: mode.rings() && tab.state == TabState::Active,
            ..tab
        })
        .collect()
}

fn scroller(
    name: &str,
    scroll: &gpui::ScrollHandle,
) -> gpui::Stateful<gpui::Div> {
    div()
        .id(id_of(name, "tabs"))
        .flex()
        .flex_1()
        .min_w_0()
        .items_center()
        .gap(px(GAP))
        .overflow_x_scroll()
        .track_scroll(scroll)
}
fn shown_of(strip: &PaneStrip) -> Option<Shown> {
    let tab = strip.tabs.get(strip.shown)?;
    let item = strip.shown + usize::from(strip.chat_tab.is_some());
    Some((tab.pane.clone(), item))
}

fn id_of(
    name: &str,
    part: &str,
) -> gpui::ElementId {
    SharedString::from(format!("pane-strip-{part}-{name}")).into()
}
fn probe(
    scroll: &gpui::ScrollHandle,
    (state, shown): (Entity<StripState>, Option<Shown>),
) -> impl IntoElement {
    let scroll = scroll.clone();
    canvas(
        move |_, _, cx: &mut App| remeasure(&state, (&scroll, shown.as_ref()), cx),
        |_, (), _, _| {},
    )
    .absolute()
    .size_0()
}

fn remeasure(
    state: &Entity<StripState>,
    (scroll, shown): (&gpui::ScrollHandle, Option<&Shown>),
    cx: &mut App,
) {
    let (spill, width) = (scroll.max_offset().x, scroll.bounds().size.width);
    let changed = state.update(cx, |state, _| {
        let resized = state.measure(spill);
        let followed = shown.is_some_and(|(pane, item)| state.follow(pane, *item, width));
        resized | followed
    });
    if !changed {
        return;
    }
    let state = state.clone();
    cx.defer(move |cx| state.update(cx, |_, cx| cx.notify()));
}

fn add_button(
    name: &str,
    on_add: TabAction,
) -> RowButton {
    RowButton {
        id: format!("add-{name}").into(),
        icon: IconName::Plus,
        tip: "New tab".into(),
        ink: RowInk::Muted,
        on_click: on_add,
    }
}

fn more_tabs(
    name: &str,
    entries: Vec<OverflowEntry>,
    palette: &Palette,
) -> impl IntoElement {
    let rows: Vec<MenuItem> = entries.into_iter().map(row_of).collect();
    let trigger = div()
        .size(px(MORE_SIZE))
        .flex()
        .items_center()
        .justify_center()
        .rounded(ui::radius::SM)
        .text_color(palette.muted_foreground())
        .hover(|trigger| trigger.bg(palette.sidebar_element_hover()))
        .debug_selector(|| "strip-more".into())
        .child(ui::Icon::new(IconName::CaretDown).size(px(MORE_GLYPH)));
    div().flex_none().child(
        ui::DropdownMenu::new(id_of(name, "more"), trigger, rows).placement(Placement::BelowEnd),
    )
}

fn row_of(entry: OverflowEntry) -> MenuItem {
    let select = entry.select;
    let action = MenuAction::new(entry.label, move |window, cx| select(window, cx));
    MenuItem::Action(match entry.state {
        TabState::Active => action.checked(),
        TabState::Idle => action,
    })
}

ui::opaque_debug!(PaneStrip);

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{
        Context, IntoElement, Modifiers, ParentElement, Render, Styled, TestAppContext, Window,
        div, px,
    };
    use ui::{Motion, Theme};

    use super::*;
    use crate::tabbar::tab_actions::{TabActions, TabMove};
    use crate::tabbar::tab_end::TabEnd;
    use crate::tabbar::tab_frame::HEIGHT as TAB_HEIGHT;
    use crate::tabbar::tab_look::TabLook;

    #[test]
    fn the_strip_row_holds_a_tab_with_its_padding() {
        assert!(
            std::hint::black_box(STRIP_HEIGHT) >= TAB_HEIGHT + 2.0 * PAD_Y,
            "row too short"
        );
    }

    fn loose_tab(name: &str) -> StripTab {
        let nothing: TabAction = Rc::new(|_, _| {});
        let actions = TabActions {
            select: Rc::clone(&nothing),
            close: Rc::clone(&nothing),
            remove: nothing,
            keep: None,
        };
        StripTab {
            pane: PaneId::new(name).expect("valid"),
            look: TabLook::plain(format!("A rather long tab title {name}")),
            state: TabState::Idle,
            end: TabEnd::Close,
            menu: Rc::from(Vec::new()),
            actions,
            ringed: false,
        }
    }

    fn entry_named(name: &str) -> OverflowEntry {
        OverflowEntry {
            label: name.to_owned().into(),
            state: TabState::Idle,
            select: Rc::new(|_, _| {}),
        }
    }

    fn chat_tab(select: &TabAction) -> ChatTab {
        ChatTab {
            pane: PaneId::new("c").expect("valid"),
            label: "Chat".into(),
            icon: IconName::ChatsCircle,
            artwork: None,
            state: TabState::Idle,
            select: Rc::clone(select),
        }
    }

    type Moves = Rc<std::cell::RefCell<Vec<(String, usize)>>>;

    fn strip_of(
        tabs: usize,
        shown: usize,
        moves: &Moves,
    ) -> PaneStrip {
        let nothing: TabAction = Rc::new(|_, _| {});
        let moves = Rc::clone(moves);
        let reorder: TabMove = Rc::new(move |pane, index, _, _| {
            moves.borrow_mut().push((pane.as_str().to_owned(), index));
        });
        let names: Vec<String> = (0..tabs).map(|index| format!("t{index}")).collect();
        let toggle = SplitToggle {
            key: "t0".into(),
            view: layout::EditorView::Open,
            on_toggle: Rc::clone(&nothing),
        };
        PaneStrip {
            key: PaneId::new("t0").expect("valid"),
            toggle,
            chat_tab: (tabs > 0).then(|| chat_tab(&nothing)),
            tabs: names.iter().map(|name| loose_tab(name)).collect(),
            shown,
            reorder,
            add: nothing,
            keys: Rc::new(|_, _, _| {}),
            overflow: names.iter().map(|name| entry_named(name)).collect(),
            review: Some(Rc::new(|_, _| {})),
        }
    }

    struct Narrow {
        tabs: usize,
        shown: usize,
        width: f32,
        moves: Moves,
    }

    impl Render for Narrow {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div()
                .w(px(self.width))
                .child(strip_of(self.tabs, self.shown, &self.moves))
        }
    }

    fn opened(
        cx: &mut TestAppContext,
        (tabs, width): (usize, f32),
    ) -> &mut gpui::VisualTestContext {
        opened_showing(cx, (tabs, 0, width))
    }

    fn opened_showing(
        cx: &mut TestAppContext,
        shape: (usize, usize, f32),
    ) -> &mut gpui::VisualTestContext {
        opened_with(cx, shape, Moves::default())
    }

    fn opened_with(
        cx: &mut TestAppContext,
        shape: (usize, usize, f32),
        moves: Moves,
    ) -> &mut gpui::VisualTestContext {
        let (tabs, shown, width) = shape;
        cx.update(|cx| {
            ui::init(cx).expect("ui init");
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let (_, cx) = cx.add_window_view(move |_, _| Narrow {
            tabs,
            shown,
            width,
            moves,
        });
        cx.run_until_parked();
        cx
    }

    fn dragging(
        cx: &mut TestAppContext,
        steps: &[f32],
    ) -> Vec<(String, usize)> {
        let moves = Moves::default();
        let cx = opened_with(cx, (3, 0, 1200.0), Rc::clone(&moves));
        let start = cx
            .debug_bounds("strip-tab-frame-t0")
            .unwrap_or_default()
            .center();
        let left = gpui::MouseButton::Left;
        let at = |step: f32| gpui::point(start.x + px(step), start.y);
        cx.simulate_mouse_down(start, left, Modifiers::none());
        for step in steps {
            cx.simulate_mouse_move(at(*step), Some(left), Modifiers::none());
        }
        let end = steps.last().copied().unwrap_or_default();
        cx.simulate_mouse_up(at(end), left, Modifiers::none());
        moves.take()
    }

    type Case = (&'static str, &'static [f32], Vec<(String, usize)>);

    #[gpui::test]
    fn a_tab_dragged_along_its_bar_lands_on_the_closest_slot(cx: &mut TestAppContext) {
        let past_both = vec![("t0".to_owned(), 2)];
        let cases: [Case; 4] = [
            ("past t1 and t2", &[210.0, 420.0], past_both.clone()),
            (
                "one fast move over t1, never hovering it",
                &[3.0, 420.0],
                past_both.clone(),
            ),
            ("a single jump past both", &[420.0], past_both),
            ("less than six pixels is still a press", &[2.0, 4.0], vec![]),
        ];
        for (name, steps, expected) in cases {
            assert_eq!(dragging(cx, steps), expected, "{name}");
        }
    }

    #[gpui::test]
    fn the_more_tabs_caret_appears_only_when_the_tabs_outgrow_the_row(cx: &mut TestAppContext) {
        let crowded = opened(cx, (12, 300.0)).debug_bounds("strip-more").is_some();
        assert!(crowded, "twelve tabs do not fit in 300 px");
    }

    #[gpui::test]
    fn a_strip_that_fits_has_no_caret(cx: &mut TestAppContext) {
        let roomy = opened(cx, (2, 800.0)).debug_bounds("strip-more").is_some();
        assert!(!roomy, "two tabs fit in 800 px");
    }

    #[gpui::test]
    fn the_more_tabs_menu_ends_where_its_caret_ends(cx: &mut TestAppContext) {
        let cx = opened(cx, (12, 400.0));
        let caret = cx.debug_bounds("strip-more").unwrap_or_default();
        cx.simulate_click(caret.center(), Modifiers::none());
        cx.run_until_parked();
        let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
        assert!(
            (menu.right() - caret.right()).abs() < px(0.5),
            "{menu:?} ends with {caret:?}"
        );
    }

    #[test]
    fn the_bar_sits_on_the_pane_s_own_ground_like_the_editor_below_it() {
        for mode in [ui::ThemeMode::Dark, ui::ThemeMode::Light] {
            let palette = Theme::default().with_mode(mode).palette(Surface::Window);
            assert_eq!(row_fill(palette), palette.pane_background(), "{mode:?}");
        }
    }

    #[gpui::test]
    fn the_shown_tab_is_scrolled_fully_into_view_past_the_chat_tab(cx: &mut TestAppContext) {
        let width = 400.0;
        let cx = opened_showing(cx, (4, 3, width));
        let shown = cx.debug_bounds("strip-tab-frame-t3").unwrap_or_default();
        let caret = cx.debug_bounds("strip-more").unwrap_or_default();
        assert!(shown.left() >= px(0.0), "{shown:?} starts in the row");
        assert!(
            shown.right() < caret.left(),
            "{shown:?} ends before {caret:?}"
        );
    }

    #[gpui::test]
    fn a_bar_with_no_tab_holds_only_the_split_toggle_and_the_review_shortcut(
        cx: &mut TestAppContext
    ) {
        let cx = opened(cx, (0, 800.0));
        assert!(
            cx.debug_bounds("row-button-split-t0").is_some(),
            "the toggle"
        );
        assert!(cx.debug_bounds("row-button-add-t0").is_none(), "no +");
        assert!(cx.debug_bounds("chat-tab-c").is_none(), "no chat tab");
        assert!(
            cx.debug_bounds("row-button-review-t0").is_some(),
            "the review shortcut still ends the row"
        );
    }

    #[gpui::test]
    fn the_toggle_leads_then_the_chat_tab_then_the_tabs_then_the_plus(cx: &mut TestAppContext) {
        let cx = opened(cx, (2, 800.0));
        let parts = [
            "row-button-split-t0",
            "chat-tab-c",
            "strip-tab-frame-t0",
            "row-button-add-t0",
            "row-button-review-t0",
        ];
        let order = parts.map(|part| cx.debug_bounds(part).map(|bounds| bounds.left()));
        assert!(order.iter().all(Option::is_some), "{order:?}");
        assert!(order.is_sorted(), "{order:?}");
    }
}
