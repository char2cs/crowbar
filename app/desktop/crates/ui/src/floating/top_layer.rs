//! The overlay window above an owner window that paints every floating element of the owner,
//! so dialogs, menus, tooltips and toasts float above native web views instead of under them.

use std::rc::Rc;

use gpui::{
    AnyWindowHandle, App, AppContext as _, Bounds, Context, IntoElement, ParentElement, Point,
    Render, Styled, Window, WindowBackgroundAppearance, WindowBounds, WindowOptions, div,
};

use crate::error::Error;
use crate::foundation::opaque_debug;

mod layer;
mod layers;
mod reach;
mod redraw;
mod registry;
mod slot;

pub(crate) use layer::{Build, Layer};
pub(crate) use reach::Reach;
pub(crate) use registry::Registry;
pub(crate) use slot::Slot;

use layers::Layers;

/// Root view of the overlay window of one owner window; it paints what the owner's
/// [`crate::Portal`]s show, in priority order, over the owner's content area.
pub struct TopLayer {
    layers: Rc<Layers>,
}

impl TopLayer {
    /// Opens the top layer of `owner`: a transparent window the platform keeps over the
    /// owner's content area, that only takes the pointer where a floating element is and the
    /// keyboard while one of them is focused. It closes with its owner.
    ///
    /// # Errors
    ///
    /// Returns [`Error::TopLayer`] when `owner` is gone or the platform cannot open the window.
    pub fn open(
        owner: AnyWindowHandle,
        cx: &mut App,
    ) -> Result<AnyWindowHandle, Error> {
        let bounds = owner
            .update(cx, |_, window, _| window.bounds())
            .map_err(Error::TopLayer)?;
        let layers = Rc::new(Layers::default());
        let root = Rc::clone(&layers);
        let window = cx
            .open_window(options(owner, bounds), move |_, cx| {
                cx.new(|_| Self { layers: root })
            })
            .map_err(Error::TopLayer)?;
        let window: AnyWindowHandle = window.into();
        layers.attach(window);
        let owner_id = owner.window_id();
        Registry::insert(owner_id, layers, cx);
        owner
            .update(cx, |_, window, _| window.refresh())
            .map_err(Error::TopLayer)?;
        cx.on_window_closed(move |cx, _| close_with_owner(owner, window, cx))
            .detach();
        Ok(window)
    }

    /// Whether the top layer of `owner` holds the keyboard, because one of its floating
    /// elements is focused; the owner then leaves the keyboard where it is.
    pub fn holds_keys(
        owner: &Window,
        cx: &mut App,
    ) -> bool {
        Registry::of(owner, cx).is_some_and(|layers| layers.take_keys(cx))
    }
}

fn options(
    owner: AnyWindowHandle,
    bounds: Bounds<gpui::Pixels>,
) -> WindowOptions {
    WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        titlebar: None,
        focus: false,
        show: true,
        is_movable: false,
        is_resizable: false,
        is_minimizable: false,
        window_background: WindowBackgroundAppearance::Transparent,
        overlay_of: Some(owner),
        ..WindowOptions::default()
    }
}

fn close_with_owner(
    owner: AnyWindowHandle,
    window: AnyWindowHandle,
    cx: &mut App,
) {
    if cx.windows().contains(&owner) {
        return;
    }
    Registry::remove(owner.window_id(), cx);
    if let Err(error) = window.update(cx, |_, window, _| window.remove_window()) {
        tracing::debug!(%error, "the top layer was already closed");
    }
}

impl Render for TopLayer {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let owner = window
            .owner_bounds()
            .unwrap_or_else(|| Bounds::new(Point::default(), window.viewport_size()));
        let layers: Vec<_> = self
            .layers
            .shown()
            .iter()
            .map(|layer| layer.paint(window, cx))
            .collect();
        div().size_full().relative().child(
            div()
                .absolute()
                .left(owner.origin.x)
                .top(owner.origin.y)
                .w(owner.size.width)
                .h(owner.size.height)
                .children(layers),
        )
    }
}

opaque_debug!(TopLayer);

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use gpui::{
        Entity, FocusHandle, InteractiveElement, Render, StatefulInteractiveElement,
        TestAppContext, VisualTestContext, px,
    };

    use super::*;
    use crate::floating::Portal;
    use crate::test_support::probe::{open_floating, settle};

    fn focusable(focus: &FocusHandle) -> gpui::Div {
        div().size(px(10.0)).track_focus(focus)
    }

    fn counting(
        focus: &FocusHandle,
        pressed: &Rc<Cell<usize>>,
    ) -> gpui::Div {
        let count = Rc::clone(pressed);
        focusable(focus).on_key_down(move |_, _, _| count.set(count.get() + 1))
    }

    #[gpui::test]
    fn an_element_the_owner_focuses_in_the_top_layer_takes_the_keyboard(cx: &mut TestAppContext) {
        let focus = cx.update(|cx| cx.focus_handle());
        let pressed = Rc::new(Cell::new(0));
        let (inner, count) = (focus.clone(), Rc::clone(&pressed));
        let (mut owner, mut top) = open_floating(cx, move |_, _| {
            let (inner, count) = (inner.clone(), Rc::clone(&count));
            Portal::here("p", move |_, _| counting(&inner, &count))
        });
        assert!(
            !top.update(|window, _| window.takes_keys()),
            "nothing focused yet"
        );
        owner.update(|window, cx| window.focus(&focus, cx));
        owner.run_until_parked();
        assert!(top.update(|window, _| window.takes_keys()));
        assert!(
            owner.update(|window, cx| TopLayer::holds_keys(window, cx)),
            "the owner sees it"
        );
        top.simulate_keystrokes("a");
        assert_eq!(pressed.get(), 1);
    }

    #[gpui::test]
    fn focus_given_to_an_element_of_the_owner_goes_back_to_the_owner(cx: &mut TestAppContext) {
        let focus = cx.update(|cx| cx.focus_handle());
        let inner = focus.clone();
        let (mut owner, mut top) = open_floating(cx, move |_, _| focusable(&inner));
        top.update(|window, cx| window.focus(&focus, cx));
        top.run_until_parked();
        assert!(!top.update(|window, _| window.takes_keys()));
        assert!(owner.update(|window, _| focus.is_focused(window)));
    }

    struct Toggled {
        focus: FocusHandle,
        back: FocusHandle,
        showing: Rc<Cell<bool>>,
        pressed: Rc<Cell<usize>>,
        back_pressed: Rc<Cell<usize>>,
    }

    fn toggled(cx: &mut TestAppContext) -> (Toggled, VisualTestContext, VisualTestContext) {
        let (focus, back) = cx.update(|cx| (cx.focus_handle(), cx.focus_handle()));
        let toggled = Toggled {
            focus: focus.clone(),
            back: back.clone(),
            showing: Rc::new(Cell::new(true)),
            pressed: Rc::new(Cell::new(0)),
            back_pressed: Rc::new(Cell::new(0)),
        };
        let (showing, pressed) = (Rc::clone(&toggled.showing), Rc::clone(&toggled.pressed));
        let back_pressed = Rc::clone(&toggled.back_pressed);
        let (owner, top) = open_floating(cx, move |_, _| {
            let (inner, count) = (focus.clone(), Rc::clone(&pressed));
            let portal = Portal::here("p", move |_, _| counting(&inner, &count));
            counting(&back, &back_pressed).children(showing.get().then_some(portal))
        });
        (toggled, owner, top)
    }

    fn close(
        toggled: &Toggled,
        owner: &mut VisualTestContext,
    ) {
        toggled.showing.set(false);
        owner.update(|window, cx| window.focus(&toggled.back, cx));
        settle(owner);
    }

    #[gpui::test]
    fn a_key_typed_as_a_floating_element_closes_reaches_the_owners_new_focus(
        cx: &mut TestAppContext
    ) {
        let (toggled, mut owner, mut top) = toggled(cx);
        owner.update(|window, cx| window.focus(&toggled.focus, cx));
        owner.run_until_parked();
        assert!(top.update(|window, _| window.takes_keys()), "opened");
        toggled.showing.set(false);
        owner.update(|window, cx| window.focus(&toggled.back, cx));
        top.simulate_keystrokes("a");
        settle(&mut owner);
        assert_eq!(toggled.pressed.get(), 0, "the closed element takes nothing");
        assert_eq!(
            toggled.back_pressed.get(),
            1,
            "the early key reaches the owner's focus"
        );
    }

    #[gpui::test]
    fn a_top_layer_hidden_as_a_row_focuses_the_owner_hands_it_the_keyboard_without_drawing(
        cx: &mut TestAppContext
    ) {
        let (toggled, mut owner, mut top) = toggled(cx);
        owner.update(|window, cx| window.focus(&toggled.focus, cx));
        owner.run_until_parked();
        assert!(top.update(|window, _| window.takes_keys()), "opened");
        let (takes_keys, focused) = top.update(|window, cx| {
            window.focus(&toggled.back, cx);
            window.set_overlay_shown(false, cx);
            (window.takes_keys(), window.focused(cx).is_some())
        });
        assert!(
            !takes_keys,
            "a hidden window draws no frame to give up the keyboard in"
        );
        assert!(!focused, "nor one to hand the focus back in");
        owner.run_until_parked();
        let back = owner.update(|window, _| toggled.back.is_focused(window));
        assert!(back, "the owner's field the row focused");
        let held = owner.update(|window, cx| TopLayer::holds_keys(window, cx));
        assert!(!held, "the owner hears the keyboard come back");
    }

    #[gpui::test]
    fn a_floating_element_opened_again_takes_the_keyboard_again(cx: &mut TestAppContext) {
        let (toggled, mut owner, mut top) = toggled(cx);
        owner.update(|window, cx| window.focus(&toggled.focus, cx));
        owner.run_until_parked();
        toggled.showing.set(false);
        owner.update(|window, _| window.refresh());
        owner.run_until_parked();
        toggled.showing.set(true);
        owner.update(|window, cx| window.focus(&toggled.focus, cx));
        owner.update(|window, _| window.refresh());
        owner.run_until_parked();
        assert!(
            top.update(|window, _| window.takes_keys()),
            "the second open"
        );
        top.simulate_keystrokes("a");
        assert_eq!(toggled.pressed.get(), 1);
    }

    #[gpui::test]
    fn the_owner_keeps_the_focus_it_restores_when_a_floating_element_closes(
        cx: &mut TestAppContext
    ) {
        let (toggled, mut owner, mut top) = toggled(cx);
        owner.update(|window, cx| window.focus(&toggled.focus, cx));
        owner.run_until_parked();
        close(&toggled, &mut owner);
        assert!(!top.update(|window, _| window.takes_keys()), "closed");
        let restored = owner.update(|window, _| toggled.back.is_focused(window));
        assert!(restored, "the owner's restore stands");
    }

    #[gpui::test]
    fn the_top_layer_closes_with_its_owner(cx: &mut TestAppContext) {
        let (mut owner, _) = open_floating(cx, |_, _| div());
        owner.update(|window, _| window.remove_window());
        owner.run_until_parked();
        assert!(cx.update(|cx| cx.windows().is_empty()));
    }

    #[derive(Clone)]
    struct Carried;

    struct Ghost;

    impl Render for Ghost {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            div().size(px(4.0))
        }
    }

    fn grip() -> Portal {
        Portal::here("grip", |_, _| {
            div()
                .id("grip")
                .size(px(10.0))
                .on_drag(Carried, |_, _, _, cx| cx.new(|_| Ghost))
        })
    }

    fn drop_target(dropped: &Rc<Cell<usize>>) -> gpui::Stateful<gpui::Div> {
        let count = Rc::clone(dropped);
        div()
            .id("target")
            .size(px(300.0))
            .on_drop(move |_: &Carried, _, _| count.set(count.get() + 1))
            .child(div().size(px(10.0)).child(grip()))
    }

    #[gpui::test]
    fn a_drag_from_the_top_layer_drops_on_the_owner(cx: &mut TestAppContext) {
        let dropped = Rc::new(Cell::new(0));
        let count = Rc::clone(&dropped);
        let (_owner, mut top) = open_floating(cx, move |_, _| drop_target(&count));
        let (left, none) = (gpui::MouseButton::Left, gpui::Modifiers::none());
        top.simulate_mouse_down(gpui::point(px(5.0), px(5.0)), left, none);
        top.simulate_mouse_move(gpui::point(px(100.0), px(100.0)), left, none);
        top.simulate_mouse_move(gpui::point(px(120.0), px(100.0)), left, none);
        top.simulate_mouse_up(gpui::point(px(120.0), px(100.0)), left, none);
        top.run_until_parked();
        assert_eq!(dropped.get(), 1, "the owner holds the drop target");
        assert!(!cx.update(|cx| cx.has_active_drag()), "the drag ended");
    }

    fn floating_drop_target(dropped: &Rc<Cell<usize>>) -> Portal {
        let count = Rc::clone(dropped);
        Portal::here("panel", move |_, _| panel(&count))
    }

    fn panel(seen: &Rc<Cell<usize>>) -> gpui::Stateful<gpui::Div> {
        let (moved, dropped) = (Rc::clone(seen), Rc::clone(seen));
        div()
            .id("panel")
            .size(px(100.0))
            .on_drag_move(move |_: &gpui::DragMoveEvent<Carried>, _, _| {
                moved.set(moved.get() + 10);
            })
            .on_drop(move |_: &Carried, _, _| dropped.set(dropped.get() + 1))
            .child(
                div()
                    .id("grip")
                    .size(px(10.0))
                    .on_drag(Carried, |_, _, _, cx| cx.new(|_| Ghost)),
            )
    }

    #[gpui::test]
    fn a_drag_inside_the_top_layer_drops_on_its_own_target(cx: &mut TestAppContext) {
        let seen = Rc::new(Cell::new(0));
        let count = Rc::clone(&seen);
        let (_owner, mut top) = open_floating(cx, move |_, _| {
            div().size(px(300.0)).child(floating_drop_target(&count))
        });
        let (left, none) = (gpui::MouseButton::Left, gpui::Modifiers::none());
        top.simulate_mouse_down(gpui::point(px(5.0), px(5.0)), left, none);
        top.simulate_mouse_move(gpui::point(px(40.0), px(50.0)), left, none);
        top.simulate_mouse_move(gpui::point(px(60.0), px(60.0)), left, none);
        top.simulate_mouse_up(gpui::point(px(60.0), px(60.0)), left, none);
        top.run_until_parked();
        assert!(
            seen.get() > 10,
            "moves reached the top layer's target: {}",
            seen.get()
        );
        assert_eq!(seen.get() % 10, 1, "the release dropped on it");
        assert!(!cx.update(|cx| cx.has_active_drag()), "the drag ended");
    }

    #[gpui::test]
    fn files_dropped_on_the_top_layer_drop_on_the_owner(cx: &mut TestAppContext) {
        let dropped = Rc::new(Cell::new(0));
        let count = Rc::clone(&dropped);
        let (_owner, mut top) = open_floating(cx, move |_, _| {
            let count = Rc::clone(&count);
            div()
                .id("target")
                .size(px(300.0))
                .on_drop(move |_: &gpui::ExternalPaths, _, _| count.set(count.get() + 1))
                .child(Portal::here("p", |_, _| div().size(px(10.0))))
        });
        let position = gpui::point(px(100.0), px(100.0));
        let paths = gpui::ExternalPaths([std::path::PathBuf::from("/a")].into_iter().collect());
        top.simulate_event(gpui::FileDropEvent::Entered { position, paths });
        top.simulate_event(gpui::FileDropEvent::Pending { position });
        top.simulate_event(gpui::FileDropEvent::Submit { position });
        top.run_until_parked();
        assert_eq!(dropped.get(), 1, "the owner holds the drop target");
    }

    #[gpui::test]
    fn a_key_typed_before_the_top_layer_draws_reaches_the_focused_element(cx: &mut TestAppContext) {
        let (toggled, mut owner, mut top) = toggled(cx);
        let key = gpui::Keystroke::parse("a").unwrap_or_else(|error| panic!("{error}"));
        owner.update(|window, cx| {
            window.focus(&toggled.focus, cx);
            window.dispatch_keystroke(key, cx);
        });
        owner.run_until_parked();
        assert!(top.update(|window, _| window.takes_keys()), "opened");
        assert_eq!(toggled.pressed.get(), 1, "the early key was kept for it");
    }

    struct Width(f32);

    fn sized(
        width: &Entity<Width>,
        cx: &App,
    ) -> gpui::Div {
        div().w(px(width.read(cx).0))
    }

    #[gpui::test]
    fn an_entity_the_top_layer_draws_stays_in_its_owner_window(cx: &mut TestAppContext) {
        let width = cx.new(|_| Width(1.0));
        let read = width.clone();
        let (mut owner, mut top) = open_floating(cx, move |_, cx| {
            let read = read.clone();
            let shown = sized(&read, cx);
            div()
                .child(shown)
                .child(Portal::here("p", move |_, cx| sized(&read, cx)))
        });
        top.run_until_parked();
        let home = width.update_in(&mut top, |_, window, _| window.window_handle());
        let owner = owner.update(|window, _| window.window_handle());
        assert_eq!(home.window_id(), owner.window_id());
    }
}
