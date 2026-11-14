//! A background that fades in while the pointer is over an element.

use gpui::{
    App, Div, ElementId, Hsla, InteractiveElement, Stateful, StatefulInteractiveElement, Styled,
    Window,
};

use crate::hover;
use crate::pose::Pose;

/// Gives `base` the id `id` and a background that fades from transparent to `color` over 150ms on hover.
pub fn hover_fill(
    base: Div,
    id: impl Into<ElementId>,
    color: Hsla,
    (window, cx): (&mut Window, &mut App),
) -> Stateful<Div> {
    let id = id.into();
    let state = hover::track(&id, Pose::Off, window, cx);
    let shown = hover::color(&state, (color.opacity(0.0), color), window, cx);
    base.id(id).bg(shown).on_hover(hover::listener(&state))
}

#[cfg(test)]
#[path = "hover_fill_tests.rs"]
mod tests;
