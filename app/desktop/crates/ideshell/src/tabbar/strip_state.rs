use gpui::{App, Context, Entity, FocusHandle, Pixels, Point, ScrollHandle, Size, Window, px};
use protocol::PaneId;

use crate::tabbar::input_mode::InputMode;
use crate::tabbar::strip_drag::StripDrag;
use crate::tabbar::tab_actions::TabAction;

const OVERFLOW_SLACK: f32 = 1.0;
pub(crate) struct StripState {
    pub(crate) scroll: ScrollHandle,
    pub(crate) focus: FocusHandle,
    pub(crate) overflowing: bool,
    pub(crate) followed: Option<(PaneId, Pixels)>,
    pub(crate) mode: InputMode,
    pub(crate) pressed: Point<Pixels>,
    pub(crate) drag: Option<StripDrag>,
    pub(crate) chosen: Option<TabAction>,
}

impl StripState {
    pub(crate) fn use_mode(
        state: &Entity<Self>,
        mode: InputMode,
        cx: &mut App,
    ) {
        state.update(cx, |state, cx| state.switch_to(mode, cx));
    }

    fn switch_to(
        &mut self,
        mode: InputMode,
        cx: &mut Context<'_, Self>,
    ) {
        if self.mode == mode {
            return;
        }
        self.mode = mode;
        cx.notify();
    }

    pub(crate) fn shown_mode(
        state: &Entity<Self>,
        focus: &FocusHandle,
        (window, cx): (&Window, &App),
    ) -> InputMode {
        if focus.is_focused(window) {
            return state.read(cx).mode;
        }
        InputMode::Pointer
    }

    pub(crate) fn new(cx: &mut Context<'_, Self>) -> Self {
        Self {
            scroll: ScrollHandle::new(),
            focus: cx.focus_handle(),
            overflowing: false,
            followed: None,
            mode: InputMode::default(),
            pressed: Point::default(),
            drag: None,
            chosen: None,
        }
    }

    pub(crate) fn begin(
        &mut self,
        tab: (PaneId, usize),
        tabs: usize,
    ) {
        let children = self.scroll.children_count();
        let lead = children.saturating_sub(tabs);
        let centres = (lead..children)
            .filter_map(|child| self.scroll.bounds_for_item(child))
            .map(|bounds| bounds.center().x)
            .collect();
        self.drag = Some(StripDrag::new(tab, self.pressed, centres));
    }

    pub(crate) fn tab_size(
        &self,
        (index, tabs): (usize, usize),
    ) -> Option<Size<Pixels>> {
        let lead = self.scroll.children_count().saturating_sub(tabs);
        self.scroll
            .bounds_for_item(lead + index)
            .map(|bounds| bounds.size)
    }

    pub(crate) fn dragged(
        &mut self,
        pointer: Point<Pixels>,
    ) -> bool {
        self.drag.as_mut().is_some_and(|drag| drag.moved(pointer))
    }

    pub(crate) fn chosen_once_armed(&mut self) -> Option<TabAction> {
        let armed = self.drag.as_ref().is_some_and(StripDrag::armed);
        armed.then(|| self.chosen.take()).flatten()
    }

    pub(crate) fn land(
        &mut self,
        pointer: Point<Pixels>,
    ) -> Option<(PaneId, usize)> {
        let mut drag = self.drag.take()?;
        drag.moved(pointer);
        drag.landing()
    }
    pub(crate) fn follow(
        &mut self,
        pane: &PaneId,
        index: usize,
        width: Pixels,
    ) -> bool {
        let seen = Some((pane.clone(), width));
        if self.followed == seen {
            return false;
        }
        self.scroll.scroll_to_item(index);
        self.followed = seen;
        true
    }
    pub(crate) fn measure(
        &mut self,
        spill: Pixels,
    ) -> bool {
        let overflowing = spill > px(OVERFLOW_SLACK);
        let changed = self.overflowing != overflowing;
        self.overflowing = overflowing;
        changed
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn the_shown_tab_is_followed_again_only_when_it_or_the_bar_s_width_changes(
        cx: &mut TestAppContext
    ) {
        let state = cx.update(|cx| cx.new(StripState::new));
        let (first, second) = (PaneId::new("a").unwrap(), PaneId::new("b").unwrap());
        state.update(cx, |state, _| {
            let follows = [
                state.follow(&first, 0, px(300.0)),
                state.follow(&first, 3, px(300.0)),
                state.follow(&second, 1, px(300.0)),
                state.follow(&second, 1, px(270.0)),
            ];
            assert_eq!(
                follows,
                [true, false, true, true],
                "new tab, same, new tab, narrower"
            );
            assert!(state.measure(px(40.0)), "now overflowing");
            assert!(!state.measure(px(40.0)), "still overflowing");
            assert!(
                state.measure(px(0.5)),
                "half a pixel is rounding, it fits again"
            );
        });
    }
}
