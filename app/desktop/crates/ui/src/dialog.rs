//! Modal application dialog with scrim.

use gpui::prelude::FluentBuilder;
use gpui::{
    Animation, AnimationExt, AnyElement, App, Bounds, ElementId, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, SharedString, Styled, Window, anchored, deferred, div, hsla, point, px,
};

use crate::button::Button;
use crate::button_size::ButtonSize;
use crate::button_variant::ButtonVariant;
use crate::click_handler::ClickHandler;
use crate::cubic_bezier::CubicBezier;
use crate::dialog_size::DialogSize;
use crate::font::{HEADING_FAMILY, UI_FAMILY};
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::layer;
use crate::motion::Motion;
use crate::overlay_slot::OverlaySlot;
use crate::overlays::Overlays;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size;
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
    title: Option<SharedString>,
    icon: Option<IconName>,
    size: DialogSize,
    header_actions: Option<AnyElement>,
    body: Option<AnyElement>,
    footer: Option<AnyElement>,
    on_close: Option<ClickHandler>,
}

impl Dialog {
    /// An empty dialog; render it at the root of the window while it is open.
    pub fn new(id: impl Into<ElementId>) -> Self {
        Self {
            id: id.into(),
            title: None,
            icon: None,
            size: DialogSize::default(),
            header_actions: None,
            body: None,
            footer: None,
            on_close: None,
        }
    }

    /// Sets the heading.
    #[must_use]
    pub fn title(mut self, title: impl Into<SharedString>) -> Self {
        self.title = Some(title.into());
        self
    }

    /// Shows an icon before the title.
    #[must_use]
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
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
        self.header_actions = Some(actions.into_any_element());
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

    /// Registers the handler for the close button, `Escape` and a press on the scrim.
    #[must_use]
    pub fn on_close(mut self, handler: impl Fn(&gpui::ClickEvent, &mut Window, &mut App) + 'static) -> Self {
        self.on_close = Some(Box::new(handler));
        self
    }
}

fn spacer(share: f32) -> gpui::Div {
    let mut spacer = div();
    spacer.style().flex_grow = Some(share);
    spacer.style().flex_basis = Some(px(0.0).into());
    spacer
}

impl RenderOnce for Dialog {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let viewport = window.viewport_size();
        let slot = window.use_keyed_state((self.id.clone(), "overlay"), cx, |_, cx| OverlaySlot {
            guard: Overlays::open(cx, Bounds::new(point(px(0.0), px(0.0)), viewport)),
        });
        slot.read(cx).guard.update(Bounds::new(point(px(0.0), px(0.0)), viewport));
        let close = std::rc::Rc::new(self.on_close);
        let header = (self.title.is_some() || self.icon.is_some() || self.header_actions.is_some()).then(|| {
            let close = close.clone();
            div()
                .flex()
                .flex_none()
                .items_center()
                .gap(px(8.0))
                .px(px(16.0))
                .py(px(12.0))
                .border_b_1()
                .border_color(palette.border())
                .children(self.icon.map(|icon| Icon::new(icon).color(palette.muted_foreground())))
                .children(self.title.map(|title| {
                    div().font_family(HEADING_FAMILY).text_size(text_size::SM).line_height(px(14.0)).child(title)
                }))
                .children(self.header_actions.map(|actions| div().ml(px(8.0)).flex().items_center().gap(px(8.0)).child(actions)))
                .child(
                    div().ml_auto().child(
                        Button::icon_only("dialog-close", IconName::X)
                            .variant(ButtonVariant::Ghost)
                            .size(ButtonSize::Sm)
                            .when_some(close.as_ref().as_ref().map(|_| ()), |button, ()| {
                                let close = close.clone();
                                button.on_click(move |event, window, cx| {
                                    if let Some(handler) = close.as_ref() {
                                        handler(event, window, cx);
                                    }
                                })
                            }),
                    ),
                )
        });
        let popup = div()
            .id((self.id.clone(), "popup"))
            .occlude()
            .flex()
            .flex_col()
            .w_full()
            .max_w(self.size.max_width())
            .max_h(viewport.height - px(2.0 * VIEWPORT_PADDING))
            .overflow_hidden()
            .rounded(radius::XL)
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(UI_FAMILY)
            .shadow(shadow::lg(0.05))
            .children(header)
            .children(self.body.map(|body| div().flex_1().min_h_0().p(px(16.0)).child(body)))
            .children(self.footer.map(|footer| {
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
            }));
        let motion = Motion::of(cx);
        let scrim_close = close.clone();
        let layer = div()
            .id((self.id.clone(), "scrim"))
            .occlude()
            .flex()
            .flex_col()
            .items_center()
            .w(viewport.width)
            .h(viewport.height)
            .p(px(VIEWPORT_PADDING))
            .bg(hsla(0.0, 0.0, 0.0, SCRIM))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                if let Some(handler) = scrim_close.as_ref() {
                    handler(&gpui::ClickEvent::default(), window, cx);
                }
            })
            .child(spacer(TOP_SHARE))
            .child(popup.on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation()))
            .child(spacer(BOTTOM_SHARE))
            .with_animation(
                (self.id, "fade"),
                Animation::new(motion.duration(timing::SLOW)).with_easing(std::rc::Rc::new(|t| CubicBezier::DEFAULT.ease(t))),
                |layer, delta| layer.opacity(delta),
            );
        deferred(anchored().position(point(px(0.0), px(0.0))).child(layer)).with_priority(layer::OVERLAY)
    }
}

#[cfg(test)]
#[path = "dialog_tests.rs"]
mod tests;
