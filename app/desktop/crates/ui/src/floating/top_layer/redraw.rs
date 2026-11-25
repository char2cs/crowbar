//! One redraw of the top layer, applied once the owner's frame is done.

use gpui::{AnyWindowHandle, App, FocusHandle, Window};

pub(crate) struct Redraw {
    pub(crate) focus: Option<FocusHandle>,
    pub(crate) floating: bool,
}

pub(crate) fn show_at_once(
    window: AnyWindowHandle,
    cx: &mut App,
) {
    if let Err(error) = window.update(cx, |_, window, cx| window.set_overlay_shown(true, cx)) {
        tracing::debug!(%error, "the top layer is drawing; its redraw shows it");
    }
}

impl Redraw {
    pub(crate) fn on(
        self,
        window: AnyWindowHandle,
        cx: &mut App,
    ) {
        if let Err(error) = window.update(cx, |_, window, cx| self.apply(window, cx)) {
            tracing::debug!(%error, "the top layer closed before it could redraw");
        }
    }

    fn apply(
        self,
        window: &mut Window,
        cx: &mut App,
    ) {
        window.set_overlay_shown(self.floating, cx);
        let moved_within = |focus: &FocusHandle| focus.contains_focused(window, cx);
        if let Some(focus) = self.focus.as_ref().filter(|focus| !moved_within(focus)) {
            window.focus(focus, cx);
        }
        window.refresh();
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{
        InteractiveElement, ParentElement, Styled, TestAppContext, VisualTestContext, div, px,
    };

    use super::*;
    use crate::floating::Portal;
    use crate::test_support::probe::{open_floating, settle};

    const GHOST_SIDE: f32 = 10.0;

    fn counting(
        focus: &FocusHandle,
        pressed: &Rc<Cell<usize>>,
    ) -> gpui::Div {
        let count = Rc::clone(pressed);
        div()
            .size(px(10.0))
            .track_focus(focus)
            .on_key_down(move |_, _, _| count.set(count.get() + 1))
    }

    type Pair = (FocusHandle, FocusHandle, Rc<Cell<usize>>, Rc<Cell<usize>>);

    fn field_and_list((field, list, field_keys, list_keys): Pair) -> Portal {
        Portal::here("p", move |_, _| {
            div()
                .child(counting(&field, &field_keys))
                .child(counting(&list, &list_keys))
        })
    }

    #[gpui::test]
    fn an_element_focused_inside_the_top_layer_keeps_the_keys_when_the_owner_draws_again(
        cx: &mut TestAppContext
    ) {
        let (field, list) = cx.update(|cx| (cx.focus_handle(), cx.focus_handle()));
        let (field_keys, list_keys) = (Rc::new(Cell::new(0)), Rc::new(Cell::new(0)));
        let inner = (
            field.clone(),
            list.clone(),
            Rc::clone(&field_keys),
            Rc::clone(&list_keys),
        );
        let (mut owner, mut top) = open_floating(cx, move |_, _| field_and_list(inner.clone()));
        owner.update(|window, cx| window.focus(&field, cx));
        owner.run_until_parked();
        top.update(|window, cx| window.focus(&list, cx));
        settle(&mut top);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        top.simulate_keystrokes("escape");
        assert_eq!(
            (list_keys.get(), field_keys.get()),
            (1, 0),
            "the list keeps the keys"
        );
    }

    fn unsaved_prompt(closed: Rc<Cell<usize>>) -> Portal {
        Portal::window("dialog", move |_, _| {
            let closed = Rc::clone(&closed);
            crate::Dialog::new("d")
                .title("Unsaved Changes")
                .on_close(move |_, _| closed.set(closed.get() + 1))
        })
    }

    type Prompt = (FocusHandle, Rc<Cell<usize>>, Rc<Cell<usize>>);

    fn field_or_dialog((field, opened, closed): Prompt) -> gpui::Div {
        let dialog = (opened.get() > closed.get()).then(|| unsaved_prompt(closed));
        div()
            .child(div().size(px(10.0)).track_focus(&field))
            .children(dialog)
    }

    #[gpui::test]
    fn a_dialog_opened_over_a_focused_field_keeps_the_keys_and_closes_on_escape(
        cx: &mut TestAppContext
    ) {
        let field = cx.update(|cx| cx.focus_handle());
        let (opened, closed) = (Rc::new(Cell::new(0)), Rc::new(Cell::new(0)));
        let inner = (field.clone(), Rc::clone(&opened), Rc::clone(&closed));
        let (mut owner, mut top) = open_floating(cx, move |_, _| field_or_dialog(inner.clone()));
        owner.update(|window, cx| window.focus(&field, cx));
        settle(&mut owner);
        opened.set(1);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert!(
            top.update(|window, _| window.takes_keys()),
            "the dialog holds the keyboard, not the field under it"
        );
        top.simulate_keystrokes("escape");
        assert_eq!(closed.get(), 1, "escape closes the dialog");
    }

    fn shown_after(
        top: &mut VisualTestContext,
        redraw: Redraw,
    ) -> bool {
        let window = top.update(|window, _| window.window_handle());
        top.update(|_, cx| cx.defer(move |cx| redraw.on(window, cx)));
        top.update(|window, _| window.is_overlay_shown())
    }

    #[gpui::test]
    fn a_redraw_shows_the_top_layer_only_while_something_floats(cx: &mut TestAppContext) {
        let (_, mut top) = open_floating(cx, |_, _| div());
        let floats = Redraw {
            focus: None,
            floating: true,
        };
        assert!(shown_after(&mut top, floats), "floating: shown");
        let idle = Redraw {
            focus: None,
            floating: false,
        };
        assert!(!shown_after(&mut top, idle), "nothing floats: hidden");
    }

    fn ghost(side: Rc<Cell<f32>>) -> Portal {
        Portal::here("p", move |_, _| div().size(px(side.get())).bg(gpui::red()))
    }

    #[gpui::test]
    fn the_top_layer_reports_what_it_paints_before_that_frame_is_presented(
        cx: &mut TestAppContext
    ) {
        let painted = Rc::new(Cell::new(0.0));
        let shown = Rc::clone(&painted);
        let (_owner, mut top) = open_floating(cx, move |_, _| ghost(Rc::clone(&shown)));
        painted.set(GHOST_SIDE);
        let drawn = top.update(|window, cx| {
            window.refresh();
            window.draw(cx).clear(cx);
            window.content_region().len()
        });
        assert_eq!(
            drawn, 1,
            "the cut must cover the frame's pixels when they land"
        );
    }

    struct Floats;

    impl gpui::Render for Floats {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut gpui::Context<'_, Self>,
        ) -> impl gpui::IntoElement {
            div().child(Portal::cover("cover", |_, _| div().size_full()))
        }
    }

    fn open_with_top_layer(
        slot: Rc<Cell<Option<AnyWindowHandle>>>,
        cx: &mut App,
    ) -> gpui::private::anyhow::Result<gpui::WindowHandle<Floats>> {
        cx.open_window(gpui::WindowOptions::default(), move |window, cx| {
            build_floating(&slot, window, cx)
        })
    }

    fn build_floating(
        slot: &Cell<Option<AnyWindowHandle>>,
        window: &mut Window,
        cx: &mut App,
    ) -> gpui::Entity<Floats> {
        slot.set(crate::floating::TopLayer::open(window, cx).ok());
        gpui::AppContext::new(cx, |_| Floats)
    }

    #[gpui::test]
    fn a_top_layer_opened_with_its_owner_floats_its_first_frame_and_shares_its_input(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| cx.set_global(crate::foundation::Theme::default()));
        let top = Rc::new(Cell::new(None));
        let slot = Rc::clone(&top);
        let owner = cx.update(|cx| open_with_top_layer(slot, cx));
        assert!(
            top.get().is_some(),
            "the top layer opens while its owner is built"
        );
        let (Ok(owner), Some(top)) = (owner, top.get()) else {
            return;
        };
        let owner: AnyWindowHandle = owner.into();
        cx.run_until_parked();
        let mut top = VisualTestContext::from_window(top, cx);
        assert!(
            top.update(|window, _| window.is_overlay_shown()),
            "the cover floats"
        );
        top.simulate_keystrokes("a");
        let mut owner = VisualTestContext::from_window(owner, cx);
        assert!(
            owner.update(|window, _| window.last_input_was_keyboard()),
            "a key in the top layer is keyboard input for the owner"
        );
    }
}
