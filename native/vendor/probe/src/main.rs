//! Vendor probe: proves the pinned `gpui` and `gpui-component` actually link
//! together, not merely resolve. Both crates appear in a function signature and
//! both are exercised at runtime through `gpui_platform::application()`.
//!
//! This crate is throwaway. Delete it once `native/crates/*` depends on the
//! vendored pair for real.

use gpui::{
    App, AppContext as _, Context, Entity, IntoElement, ParentElement as _, Render, SharedString,
    Styled as _, Window, WindowOptions, div,
};
use gpui_component::{
    ActiveTheme as _, Root, StyledExt as _,
    button::{Button, ButtonVariants as _},
};

pub struct Probe {
    label: SharedString,
}

/// The load-bearing signature: a `gpui` type in, a `gpui-component` type out.
/// If the two crates were resolved from mismatched revisions this would not
/// even typecheck, let alone link.
pub fn probe_button(label: &SharedString, _window: &mut Window, cx: &mut App) -> Button {
    let _ = cx.theme().background;
    Button::new("probe").primary().label(label.clone())
}

/// A `gpui-component` type in, a `gpui` type out.
pub fn probe_root(view: Entity<Probe>, window: &mut Window, cx: &mut Context<Root>) -> Root {
    Root::new(view, window, cx)
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        div()
            .v_flex()
            .gap_2()
            .size_full()
            .items_center()
            .justify_center()
            .bg(cx.theme().background)
            .child(self.label.clone())
            .child(probe_button(&self.label, window, cx))
    }
}

fn main() {
    gpui_platform::application().run(move |cx: &mut App| {
        gpui_component::init(cx);

        cx.spawn(async move |cx| {
            cx.open_window(WindowOptions::default(), |window, cx| {
                let view = cx.new(|_| Probe {
                    label: SharedString::from("vendored gpui + gpui-component"),
                });
                cx.new(|cx| probe_root(view, window, cx).bg(cx.theme().background))
            })
            .expect("failed to open probe window");
        })
        .detach();
    });
}
