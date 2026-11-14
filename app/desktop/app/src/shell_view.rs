use gpui::{App, Bounds, Context, IntoElement, Pixels, Render, Window, canvas, div, prelude::*, px, rgb};
use webview::ParentWindow;

use crate::shell_state::ShellState;

const SIDEBAR_WIDTH: f32 = 220.0;
const TAB_BAR_HEIGHT: f32 = 36.0;

/// The window's root view: native shell chrome around the pane content area.
pub(crate) struct ShellView {
    state: ShellState,
}

impl ShellView {
    pub(crate) fn new(state: ShellState, cx: &mut Context<Self>) -> Self {
        let inbox = state.inbox();
        cx.spawn(async move |this, cx| {
            while let Ok(message) = inbox.recv().await {
                if this.update(cx, |view, _| view.state.handle_ipc(message)).is_err() {
                    break;
                }
            }
        })
        .detach();
        Self { state }
    }

    fn content_changed(&mut self, window: &Window, bounds: Bounds<Pixels>) {
        match ParentWindow::from_window(window) {
            Ok(parent) => self.state.set_content(parent, content_bounds(bounds)),
            Err(error) => tracing::error!(%error, "the window cannot host web views"),
        }
    }
}

pub(crate) fn content_bounds(bounds: Bounds<Pixels>) -> webview::Bounds {
    webview::Bounds {
        x: f64::from(f32::from(bounds.origin.x)),
        y: f64::from(f32::from(bounds.origin.y)),
        width: f64::from(f32::from(bounds.size.width)),
        height: f64::from(f32::from(bounds.size.height)),
    }
}

impl Render for ShellView {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let view = cx.entity();
        div()
            .size_full()
            .flex()
            .bg(rgb(0x0f1115))
            .text_color(rgb(0xc8ccd4))
            .child(sidebar_placeholder())
            .child(
                div()
                    .flex_1()
                    .flex()
                    .flex_col()
                    .child(tab_bar_placeholder())
                    .child(
                        canvas(
                            move |bounds, window, cx: &mut App| {
                                view.update(cx, |view, _| view.content_changed(window, bounds));
                            },
                            |_, (), _, _| {},
                        )
                        .size_full(),
                    ),
            )
    }
}

fn sidebar_placeholder() -> impl IntoElement {
    div()
        .w(px(SIDEBAR_WIDTH))
        .h_full()
        .bg(rgb(0x161920))
        .p_3()
        .child("SIDEBAR: native shell area, filled by the sidebar crate")
}

fn tab_bar_placeholder() -> impl IntoElement {
    div()
        .h(px(TAB_BAR_HEIGHT))
        .w_full()
        .bg(rgb(0x1b1f27))
        .px_3()
        .flex()
        .items_center()
        .child("TAB BAR: native shell area, filled by the tab bar crate")
}

#[cfg(test)]
#[path = "shell_view_tests.rs"]
mod tests;
