//! Placement of popups below their trigger and the click-away layer behind them.

use std::rc::Rc;

use gpui::{
    Anchor, AnchoredPositionMode, AnyElement, App, Div, ElementId, InteractiveElement, IntoElement,
    MouseButton, ParentElement, Pixels, Point, Styled, Window, anchored, div, px, relative,
};

use crate::floating::placement::Placement;
use crate::floating::portal::Portal;
use crate::foundation::tokens::layer;

const WINDOW_MARGIN: f32 = 8.0;
const SUBMENU_GAP: f32 = 2.0;
const SUBMENU_RISE: f32 = 5.0;

pub(crate) type Content = Rc<dyn Fn(&mut Window, &mut App) -> AnyElement>;

pub(crate) fn placed(
    id: ElementId,
    placement: Placement,
    offset: f32,
    content: Content,
) -> AnyElement {
    let anchor = div().absolute().top_full().mt(px(offset));
    let (anchor, corner) = match placement {
        Placement::Below => (anchor.left_0(), Anchor::TopLeft),
        Placement::BelowEnd => (anchor.right_0(), Anchor::TopRight),
        Placement::BelowCentre => (anchor.left(relative(0.5)), Anchor::TopLeft),
        Placement::Right => (
            div().absolute().left_full().top_0().ml(px(offset)),
            Anchor::TopLeft,
        ),
    };
    let portal = Portal::here(id, move |window, cx| {
        anchored()
            .anchor(corner)
            .snap_to_window_with_margin(px(WINDOW_MARGIN))
            .child(content(window, cx))
    })
    .priority(layer::MENU);
    anchor.child(portal).into_any_element()
}

pub(crate) fn over(
    id: ElementId,
    offset: Point<Pixels>,
    content: Content,
) -> AnyElement {
    let portal = Portal::here(id, move |window, cx| {
        anchored()
            .anchor(Anchor::TopLeft)
            .snap_to_window_with_margin(px(WINDOW_MARGIN))
            .child(content(window, cx))
    })
    .priority(layer::MENU);
    div()
        .absolute()
        .top(offset.y)
        .left(offset.x)
        .child(portal)
        .into_any_element()
}

pub(crate) fn beside(content: impl IntoElement) -> AnyElement {
    div()
        .absolute()
        .left_full()
        .top(px(-SUBMENU_RISE))
        .ml(px(SUBMENU_GAP))
        .child(
            anchored()
                .snap_to_window_with_margin(px(WINDOW_MARGIN))
                .child(content),
        )
        .into_any_element()
}

/// A popup in the window's top layer with its top-left corner at `position`, in window
/// coordinates; `content` is built in the window that paints it.
pub fn popup_at<E: IntoElement>(
    id: impl Into<ElementId>,
    position: Point<Pixels>,
    content: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> AnyElement {
    Portal::window(id, move |window, cx| {
        anchored()
            .position_mode(AnchoredPositionMode::Local)
            .position(position)
            .snap_to_window_with_margin(px(WINDOW_MARGIN))
            .child(content(window, cx))
    })
    .priority(layer::MENU)
    .into_any_element()
}

/// A window-sized layer in the window's top layer, behind its popups; a press on it, with
/// either button, runs `on_press`.
pub fn click_away(
    id: impl Into<ElementId>,
    on_press: impl Fn(&mut Window, &mut App) + 'static,
) -> AnyElement {
    let on_press = Rc::new(on_press);
    Portal::window(id, move |_, _| away_layer(&on_press))
        .priority(layer::MENU - 1)
        .into_any_element()
}

fn away_layer(on_press: &Rc<impl Fn(&mut Window, &mut App) + 'static>) -> Div {
    let (on_left, on_right) = (Rc::clone(on_press), Rc::clone(on_press));
    div()
        .occlude()
        .size_full()
        .on_mouse_down(MouseButton::Left, move |_, window, cx| on_left(window, cx))
        .on_mouse_down(MouseButton::Right, move |_, window, cx| {
            on_right(window, cx);
        })
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::test_support::probe::open;

    fn placed_content(placement: Placement) -> AnyElement {
        let content: Content = Rc::new(|_, _| {
            div()
                .w(px(80.0))
                .h(px(20.0))
                .debug_selector(|| "popup-content".into())
                .into_any_element()
        });
        placed("popup".into(), placement, 4.0, content)
    }

    fn tally(counter: &Rc<Cell<u32>>) -> impl Fn(&mut Window, &mut App) + 'static {
        let counter = Rc::clone(counter);
        move |_, _| counter.set(counter.get() + 1)
    }

    #[gpui::test]
    fn press_anywhere_runs_the_close_handler_once_per_press(cx: &mut TestAppContext) {
        let presses = Rc::new(Cell::new(0));
        let counter = Rc::clone(&presses);
        let cx = open(cx, move |_, _| {
            let counter = Rc::clone(&counter);
            div()
                .relative()
                .w(px(40.0))
                .h(px(20.0))
                .child(click_away("away", tally(&counter)))
        });
        cx.simulate_click(point(px(300.0), px(300.0)), gpui::Modifiers::none());
        assert_eq!(presses.get(), 1);
        cx.simulate_click(point(px(5.0), px(5.0)), gpui::Modifiers::none());
        assert_eq!(presses.get(), 2);
    }

    #[gpui::test]
    fn content_is_laid_out_right_of_the_trigger_top_aligned(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .relative()
                .w(px(100.0))
                .h(px(30.0))
                .child(placed_content(Placement::Right))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        assert!(
            content.origin.x >= trigger.right() + px(4.0) - px(0.5),
            "{content:?} right of {trigger:?}"
        );
        assert!((content.origin.y - trigger.origin.y).abs() < px(0.5));
    }

    #[gpui::test]
    fn content_is_laid_out_below_the_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .relative()
                .w(px(100.0))
                .h(px(30.0))
                .child(placed_content(Placement::Below))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        assert!(
            content.origin.y >= trigger.bottom() + px(4.0) - px(0.5),
            "{content:?} below {trigger:?}"
        );
    }

    #[gpui::test]
    fn an_end_aligned_popup_shares_the_right_edge_of_its_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .relative()
                .w(px(100.0))
                .h(px(30.0))
                .child(placed_content(Placement::BelowEnd))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        assert!(
            (content.right() - trigger.right()).abs() < px(0.5),
            "{content:?} end of {trigger:?}"
        );
        assert!(content.origin.y >= trigger.bottom() + px(4.0) - px(0.5));
    }

    #[gpui::test]
    fn a_centred_popup_starts_at_the_horizontal_centre_of_its_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .relative()
                .w(px(100.0))
                .h(px(30.0))
                .child(placed_content(Placement::BelowCentre))
        });
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let content = cx.debug_bounds("popup-content").unwrap_or_default();
        let centre = trigger.origin.x + trigger.size.width * 0.5;
        assert!(
            (content.origin.x - centre).abs() < px(0.5),
            "{content:?} from {centre:?}"
        );
        assert!((content.origin.y - (trigger.bottom() + px(4.0))).abs() < px(0.5));
    }
}
