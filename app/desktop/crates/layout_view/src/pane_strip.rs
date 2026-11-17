use gpui::{
    App, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    StatefulInteractiveElement, Styled, Window, canvas, div, px,
};
use protocol::PaneId;
use ui::{Button, ButtonSize, IconName, MenuAction, MenuItem, Palette, Surface, Theme, Tooltip};

use crate::overflow_entry::OverflowEntry;
use crate::strip_state::StripState;
use crate::strip_tab::StripTab;
use crate::tab_actions::{StripKeys, TabAction, TabDrop};
use crate::tab_state::TabState;

pub(crate) const STRIP_HEIGHT: f32 = 42.0;
const PAD_X: f32 = 8.0;
const PAD_Y: f32 = 4.0;
const GAP: f32 = 6.0;
const MORE_SIZE: f32 = 24.0;
const MORE_GLYPH: f32 = 14.0;

#[derive(IntoElement)]
pub(crate) struct PaneStrip {
    pub(crate) key: PaneId,
    pub(crate) tabs: Vec<StripTab>,
    pub(crate) shown: usize,
    pub(crate) at_end_drop: TabDrop,
    pub(crate) add: TabAction,
    pub(crate) keys: StripKeys,
    pub(crate) overflow: Vec<OverflowEntry>,
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
        follow_shown(&state, (&self.tabs, self.shown), cx);
        let (focus, scroll, overflowing) = {
            let held = state.read(cx);
            (held.focus.clone(), held.scroll.clone(), held.overflowing)
        };
        let (keys, at_end) = (self.keys, self.at_end_drop);
        let measuring = state.clone();
        let tabs = scroller(name, &scroll)
            .children(
                self.tabs
                    .into_iter()
                    .map(|tab| div().flex_none().child(tab)),
            )
            .child(add_button(name, self.add));
        let more = overflowing.then(|| more_tabs(name, self.overflow, palette));
        div()
            .id(id_of(name, "row"))
            .track_focus(&focus)
            .flex()
            .items_center()
            .gap(px(GAP))
            .px(px(PAD_X))
            .py(px(PAD_Y))
            .w_full()
            .h(px(STRIP_HEIGHT))
            .overflow_hidden()
            .bg(palette.pane_chrome_bg())
            .on_key_down(move |event, window, cx| keys(event.keystroke.key.as_str(), window, cx))
            .capture_any_mouse_down(move |_, window, _| window.focus(&focus))
            .on_drop::<model::DragSubject>(move |subject, window, cx| at_end(subject, window, cx))
            .child(tabs)
            .child(probe(&scroll, measuring))
            .children(more)
    }
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
fn follow_shown(
    state: &Entity<StripState>,
    (tabs, shown): (&[StripTab], usize),
    cx: &mut App,
) {
    let Some(pane) = tabs.get(shown).map(|tab| tab.pane.clone()) else {
        return;
    };
    state.update(cx, |state, _| state.follow(&pane, shown));
}

fn id_of(
    name: &str,
    part: &str,
) -> gpui::ElementId {
    SharedString::from(format!("pane-strip-{part}-{name}")).into()
}
fn probe(
    scroll: &gpui::ScrollHandle,
    state: Entity<StripState>,
) -> impl IntoElement {
    let scroll = scroll.clone();
    canvas(
        move |_, _, cx: &mut App| remeasure(&state, scroll.max_offset().width, cx),
        |_, (), _, _| {},
    )
    .absolute()
    .size_0()
}

fn remeasure(
    state: &Entity<StripState>,
    spill: gpui::Pixels,
    cx: &mut App,
) {
    let changed = state.update(cx, |state, _| state.measure(spill));
    if changed {
        state.update(cx, |_, cx| cx.notify());
    }
}

fn add_button(
    name: &str,
    on_add: TabAction,
) -> impl IntoElement {
    let button = Button::icon_only(id_of(name, "add"), IconName::Plus)
        .size(ButtonSize::Sm)
        .on(Surface::Window)
        .on_click(move |_, window, cx| on_add(window, cx));
    div()
        .flex_none()
        .child(Tooltip::new(id_of(name, "add-tip"), "New tab", button))
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
        .child(ui::Icon::new(IconName::CaretDown).size(px(MORE_GLYPH)));
    div()
        .flex_none()
        .child(ui::DropdownMenu::new(id_of(name, "more"), trigger, rows))
}

fn row_of(entry: OverflowEntry) -> MenuItem {
    let select = entry.select;
    let action = MenuAction::new(entry.label, move |window, cx| select(window, cx));
    MenuItem::Action(match entry.state {
        TabState::Active => action.checked(),
        TabState::Idle => action,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::strip_tab::HEIGHT as TAB_HEIGHT;

    #[test]
    fn the_strip_row_holds_a_tab_with_its_padding() {
        assert!(
            std::hint::black_box(STRIP_HEIGHT) >= TAB_HEIGHT + 2.0 * PAD_Y,
            "row too short"
        );
    }
}
