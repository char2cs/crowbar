//! Modal application dialog with scrim.

use std::rc::Rc;

use gpui::{
    Animation, AnimationExt, AnyElement, App, Bounds, ElementId, InteractiveElement, IntoElement,
    MouseButton, ParentElement, RenderOnce, SharedString, Styled, Window, anchored, deferred, div,
    hsla, point, px,
};

use crate::close_handler::CloseHandler;
use crate::cubic_bezier::CubicBezier;
use crate::dialog_extent::DialogExtent;
use crate::dialog_header::DialogHeader;
use crate::dialog_size::DialogSize;
use crate::font::UI_FAMILY;
use crate::icon_name::IconName;
use crate::layer;
use crate::motion::Motion;
use crate::opacity;
use crate::opaque_debug::opaque_debug;
use crate::overlay_slot::OverlaySlot;
use crate::overlays::Overlays;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::spacing;
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
    extent: Option<DialogExtent>,
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
            extent: None,
            body: None,
            footer: None,
            on_close: None,
        }
    }

    /// Sets the heading.
    #[must_use]
    pub fn title(
        mut self,
        title: impl Into<SharedString>,
    ) -> Self {
        self.header = self.header.title(Some(title.into()));
        self
    }

    /// Shows an icon before the title.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.header = self.header.icon(Some(icon));
        self
    }

    /// Sets the maximum width.
    #[must_use]
    pub fn size(
        mut self,
        size: DialogSize,
    ) -> Self {
        self.size = size;
        self
    }

    /// Fills `extent` of the window; the body then has no padding and the header no rule.
    #[must_use]
    pub fn flush(
        mut self,
        extent: DialogExtent,
    ) -> Self {
        self.extent = Some(extent);
        self.header = self.header.unruled();
        self
    }

    /// Adds controls right after the title.
    #[must_use]
    pub fn header_actions(
        mut self,
        actions: impl IntoElement,
    ) -> Self {
        self.header = self.header.actions(Some(actions.into_any_element()));
        self
    }

    /// Sets the body content.
    #[must_use]
    pub fn body(
        mut self,
        body: impl IntoElement,
    ) -> Self {
        self.body = Some(body.into_any_element());
        self
    }

    /// Sets the footer content, right-aligned on a muted strip.
    #[must_use]
    pub fn footer(
        mut self,
        footer: impl IntoElement,
    ) -> Self {
        self.footer = Some(footer.into_any_element());
        self
    }

    /// Registers the handler for the close button and a press on the scrim.
    #[must_use]
    pub fn on_close(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        let handler: CloseHandler = Rc::new(handler);
        self.header = self.header.on_close(Some(Rc::clone(&handler)));
        self.on_close = Some(handler);
        self
    }

    fn popup(
        self,
        theme: Theme,
        viewport: gpui::Size<gpui::Pixels>,
    ) -> gpui::Stateful<gpui::Div> {
        let palette = theme.palette(Surface::Window);
        let header = (!self.header.is_empty()).then_some(self.header);
        let footer = self.footer.map(|footer| footer_bar(footer, palette));
        let padding = self.extent.map_or(spacing::STEP_4, |_| px(0.0));
        let frame = div().id((self.id, "popup")).occlude().flex().flex_col();
        let frame = match self.extent {
            Some(extent) => {
                let fixed = extent.size(viewport);
                frame.w(fixed.width).h(fixed.height)
            },
            None => frame
                .w_full()
                .max_w(self.size.max_width())
                .max_h(viewport.height - px(2.0 * VIEWPORT_PADDING)),
        };
        frame
            .overflow_hidden()
            .rounded(radius::XL)
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(UI_FAMILY)
            .shadow(shadow::lg(opacity::PERCENT_5))
            .children(header)
            .children(
                self.body
                    .map(|body| div().flex_1().min_h_0().p(padding).child(body)),
            )
            .children(footer)
    }
}

fn footer_bar(
    footer: AnyElement,
    palette: &Palette,
) -> gpui::Div {
    div()
        .flex()
        .flex_none()
        .justify_end()
        .gap(spacing::STEP_2)
        .px(spacing::STEP_4)
        .py(spacing::STEP_3)
        .border_t_1()
        .border_color(palette.border())
        .bg(palette.muted().opacity(0.5))
        .child(footer)
}

fn spacer(share: f32) -> gpui::Div {
    let mut spacer = div();
    spacer.style().flex_grow = Some(share);
    spacer.style().flex_basis = Some(px(0.0).into());
    spacer
}

fn fire(
    handler: Option<&CloseHandler>,
    window: &mut Window,
    cx: &mut App,
) {
    if let Some(handler) = handler {
        handler(window, cx);
    }
}

fn whole_window(viewport: gpui::Size<gpui::Pixels>) -> Bounds<gpui::Pixels> {
    Bounds::new(point(px(0.0), px(0.0)), viewport)
}

impl RenderOnce for Dialog {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let viewport = window.viewport_size();
        let slot = window.use_keyed_state((self.id.clone(), "overlay"), cx, |_, cx| OverlaySlot {
            guard: Overlays::open(cx, whole_window(viewport)),
        });
        slot.read(cx).guard.update(whole_window(viewport));
        let duration = Motion::of(cx).duration(timing::SLOW);
        let (id, scrim_close) = (self.id.clone(), self.on_close.clone());
        let popup = self.popup(theme, viewport);
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
mod tests {
    use std::cell::{Cell, RefCell};
    use std::rc::Rc;

    use gpui::{Bounds, ParentElement, Pixels, TestAppContext, div, point, px};

    use super::*;
    use crate::overlay_host::OverlayHost;
    use crate::overlay_id::OverlayId;
    use crate::test_support::open;

    #[derive(Default)]
    struct Recorder {
        events: RefCell<Vec<String>>,
    }

    impl OverlayHost for Recorder {
        fn overlay_changed(
            &self,
            _: OverlayId,
            bounds: Bounds<Pixels>,
        ) {
            self.events.borrow_mut().push(format!(
                "changed {}x{}",
                f32::from(bounds.size.width),
                f32::from(bounds.size.height)
            ));
        }

        fn overlay_closed(
            &self,
            _: OverlayId,
        ) {
            self.events.borrow_mut().push("closed".into());
        }
    }

    #[gpui::test]
    fn dialog_reports_the_whole_window_to_the_host(cx: &mut TestAppContext) {
        let recorder = Rc::new(Recorder::default());
        let host = Rc::clone(&recorder);
        let cx = open(cx, |_, _| Dialog::new("d").title("Title"));
        cx.update(|_, cx| Overlays::install(host as Rc<dyn OverlayHost>, cx));
        cx.simulate_resize(gpui::size(px(640.0), px(480.0)));
        cx.run_until_parked();
        let events = recorder.events.borrow();
        assert!(
            events.iter().all(|event| event.starts_with("changed")),
            "{events:?}"
        );
    }

    #[gpui::test]
    fn close_button_and_scrim_both_call_on_close(cx: &mut TestAppContext) {
        let closed = Rc::new(Cell::new(0));
        let counter = Rc::clone(&closed);
        let cx = open(cx, move |_, _| {
            let counter = Rc::clone(&counter);
            Dialog::new("d")
                .title("Title")
                .body("Body")
                .on_close(move |_, _| counter.set(counter.get() + 1))
        });
        cx.simulate_click(point(px(2.0), px(2.0)), gpui::Modifiers::none());
        assert_eq!(closed.get(), 1, "a press on the scrim closes");
    }

    #[gpui::test]
    fn a_press_inside_the_popup_does_not_close(cx: &mut TestAppContext) {
        let closed = Rc::new(Cell::new(0));
        let counter = Rc::clone(&closed);
        let cx = open(cx, move |_, _| {
            let counter = Rc::clone(&counter);
            Dialog::new("d")
                .title("Title")
                .body(div().debug_selector(|| "body".into()).child("Body"))
                .on_close(move |_, _| counter.set(counter.get() + 1))
        });
        let body = cx.debug_bounds("body").unwrap_or_default();
        cx.simulate_click(body.center(), gpui::Modifiers::none());
        assert_eq!(closed.get(), 0);
    }

    #[gpui::test]
    fn a_flush_dialog_fills_its_extent_with_no_body_padding(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            Dialog::new("d")
                .title("Title")
                .flush(DialogExtent::new(0.5, 0.5, px(300.0), px(200.0)))
                .body(div().size_full().debug_selector(|| "body".into()))
        });
        let popup = cx.debug_bounds("body").unwrap_or_default();
        assert_eq!(
            f32::from(popup.size.width),
            300.0,
            "the body spans the popup width: no padding"
        );
    }
}
