//! Modal application dialog with scrim.

use std::rc::Rc;

use gpui::{
    Animation, AnimationExt, AnyElement, App, Bounds, ElementId, InteractiveElement, IntoElement,
    MouseButton, ParentElement, RenderOnce, SharedString, Styled, Window, anchored, deferred, div,
    hsla, point, px,
};

use crate::close_handler::CloseHandler;
use crate::cubic_bezier::CubicBezier;
use crate::dialog_header::DialogHeader;
use crate::dialog_size::DialogSize;
use crate::font::UI_FAMILY;
use crate::icon_name::IconName;
use crate::layer;
use crate::motion::Motion;
use crate::opaque_debug::opaque_debug;
use crate::overlay_slot::OverlaySlot;
use crate::overlays::Overlays;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::timing;

const SCRIM: f32 = 0.32;
const VIEWPORT_PADDING: f32 = 16.0;
const TOP_SHARE: f32 = 1.0;
const BOTTOM_SHARE: f32 = 3.0;

/// A modal dialog: scrim, header with title and close button, scrolling body and optional footer.
#[derive(IntoElement)]
pub struct Dialog {
    id: ElementId,
    header: DialogHeader,
    size: DialogSize,
    body: Option<AnyElement>,
    footer: Option<AnyElement>,
    on_close: Option<CloseHandler>,
}

impl Dialog {
    /// An empty dialog; render it at the root of the window while it is open.
    pub fn new(id: impl Into<ElementId>) -> Self {
        let id = id.into();
        Self {
            header: DialogHeader::new(id.clone()),
            id,
            size: DialogSize::default(),
            body: None,
            footer: None,
            on_close: None,
        }
    }

    /// Sets the heading.
    #[must_use]
    pub fn title(mut self, title: impl Into<SharedString>) -> Self {
        self.header = self.header.title(Some(title.into()));
        self
    }

    /// Shows an icon before the title.
    #[must_use]
    pub fn icon(mut self, icon: IconName) -> Self {
        self.header = self.header.icon(Some(icon));
        self
    }

    /// Sets the maximum width.
    #[must_use]
    pub fn size(mut self, size: DialogSize) -> Self {
        self.size = size;
        self
    }

    /// Adds controls right after the title.
    #[must_use]
    pub fn header_actions(mut self, actions: impl IntoElement) -> Self {
        self.header = self.header.actions(Some(actions.into_any_element()));
        self
    }

    /// Sets the body content.
    #[must_use]
    pub fn body(mut self, body: impl IntoElement) -> Self {
        self.body = Some(body.into_any_element());
        self
    }

    /// Sets the footer content, right-aligned on a muted strip.
    #[must_use]
    pub fn footer(mut self, footer: impl IntoElement) -> Self {
        self.footer = Some(footer.into_any_element());
        self
    }

    /// Registers the handler for the close button and a press on the scrim.
    #[must_use]
    pub fn on_close(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        let handler: CloseHandler = Rc::new(handler);
        self.header = self.header.on_close(Some(Rc::clone(&handler)));
        self.on_close = Some(handler);
        self
    }

    fn popup(self, theme: Theme, viewport_height: gpui::Pixels) -> gpui::Stateful<gpui::Div> {
        let palette = theme.palette(Surface::Window);
        let header = (!self.header.is_empty()).then_some(self.header);
        let footer = self.footer.map(|footer| {
            div()
                .flex()
                .flex_none()
                .justify_end()
                .gap(px(8.0))
                .px(px(16.0))
                .py(px(12.0))
                .border_t_1()
                .border_color(palette.border())
                .bg(palette.muted().opacity(0.5))
                .child(footer)
        });
        div()
            .id((self.id, "popup"))
            .occlude()
            .flex()
            .flex_col()
            .w_full()
            .max_w(self.size.max_width())
            .max_h(viewport_height - px(2.0 * VIEWPORT_PADDING))
            .overflow_hidden()
            .rounded(radius::XL)
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(UI_FAMILY)
            .shadow(shadow::lg(0.05))
            .children(header)
            .children(
                self.body
                    .map(|body| div().flex_1().min_h_0().p(px(16.0)).child(body)),
            )
            .children(footer)
    }
}

fn spacer(share: f32) -> gpui::Div {
    let mut spacer = div();
    spacer.style().flex_grow = Some(share);
    spacer.style().flex_basis = Some(px(0.0).into());
    spacer
}

fn fire(handler: Option<&CloseHandler>, window: &mut Window, cx: &mut App) {
    if let Some(handler) = handler {
        handler(window, cx);
    }
}

fn whole_window(viewport: gpui::Size<gpui::Pixels>) -> Bounds<gpui::Pixels> {
    Bounds::new(point(px(0.0), px(0.0)), viewport)
}

impl RenderOnce for Dialog {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let viewport = window.viewport_size();
        let slot = window.use_keyed_state((self.id.clone(), "overlay"), cx, |_, cx| OverlaySlot {
            guard: Overlays::open(cx, whole_window(viewport)),
        });
        slot.read(cx).guard.update(whole_window(viewport));
        let duration = Motion::of(cx).duration(timing::SLOW);
        let (id, scrim_close) = (self.id.clone(), self.on_close.clone());
        let popup = self.popup(theme, viewport.height);
        let layer = div()
            .id((id.clone(), "scrim"))
            .occlude()
            .flex()
            .flex_col()
            .items_center()
            .w(viewport.width)
            .h(viewport.height)
            .p(px(VIEWPORT_PADDING))
            .bg(hsla(0.0, 0.0, 0.0, SCRIM))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                fire(scrim_close.as_ref(), window, cx);
            })
            .child(spacer(TOP_SHARE))
            .child(popup.on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation()))
            .child(spacer(BOTTOM_SHARE))
            .with_animation(
                (id, "fade"),
                Animation::new(duration).with_easing(|t| CubicBezier::DEFAULT.ease(t)),
                Styled::opacity,
            );
        deferred(anchored().position(point(px(0.0), px(0.0))).child(layer))
            .with_priority(layer::OVERLAY)
    }
}

opaque_debug!(Dialog);

#[cfg(test)]
#[path = "dialog_tests.rs"]
mod tests;
