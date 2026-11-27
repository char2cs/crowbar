//! Modal application dialog with scrim.

use std::rc::Rc;

use gpui::{
    AnimationExt, AnyElement, App, Div, ElementId, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, Stateful, Styled, Window, anchored, div, point, px,
};

use crate::floating::dialog::actions::{CONTEXT, CloseDialog, FocusNext, FocusPrevious};
use crate::floating::dialog::focus::Focus;
use crate::floating::dialog::header::Header;
use crate::floating::dialog::veil::fade;
use crate::foundation::CloseHandler;
use crate::foundation::IconName;
use crate::foundation::Palette;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;

const VIEWPORT_PADDING: f32 = 16.0;
const TOP_SHARE: f32 = 1.0;
const BOTTOM_SHARE: f32 = 3.0;

/// A modal dialog: scrim, header with title and close button, scrolling body and optional footer.
///
/// Not deferred: a deferred dialog cannot hold a popover, whose panel defers too. Mount
/// it as the last child of a full-window layer so it paints over its siblings.
#[derive(IntoElement)]
pub struct Dialog {
    id: ElementId,
    header: Header,
    size: DialogSize,
    style: DialogStyle,
    extent: Option<DialogExtent>,
    body: Option<AnyElement>,
    footer: Option<AnyElement>,
    on_close: Option<CloseHandler>,
}

mod actions;
mod app_header;
mod close_button;
mod dialog_extent;
mod dialog_size;
mod dialog_style;
mod focus;
mod header;
mod header_flow;
mod heading;
mod veil;

pub(crate) use actions::bindings;
pub use dialog_extent::DialogExtent;
pub use dialog_size::DialogSize;
pub use dialog_style::DialogStyle;
pub use header_flow::HeaderFlow;

impl Dialog {
    /// An empty dialog; render it at the root of the window while it is open.
    pub fn new(id: impl Into<ElementId>) -> Self {
        let id = id.into();
        Self {
            header: Header::new(id.clone()),
            id,
            size: DialogSize::default(),
            style: DialogStyle::default(),
            extent: None,
            body: None,
            footer: None,
            on_close: None,
        }
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
    ) -> Stateful<Div> {
        let palette = theme.palette(Surface::Window);
        let (header, style) = ((!self.header.is_empty()).then_some(self.header), self.style);
        let footer = footer_bar(self.footer, (palette, style));
        let padding = self.extent.map_or(self.style.body_padding(), |_| px(0.0));
        let frame = div()
            .id((self.id, "popup"))
            .debug_selector(|| "ui-dialog".into())
            .occlude()
            .flex()
            .flex_col();
        let frame = match self.extent {
            Some(extent) => {
                let fixed = extent.size(viewport);
                frame.w(fixed.width).h(fixed.height)
            },
            None => frame
                .w(self.size.max_width())
                .max_w_full()
                .max_h(viewport.height - px(2.0 * VIEWPORT_PADDING)),
        };
        self.style
            .frame(frame, palette)
            .overflow_hidden()
            .bg(palette.popover())
            .text_color(palette.foreground())
            .font_family(theme.ui_family())
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
    footer: Option<AnyElement>,
    (palette, style): (&Palette, DialogStyle),
) -> Option<Div> {
    let footer = footer?;
    let (px_x, px_y) = style.footer_padding();
    let bar = div()
        .flex()
        .flex_none()
        .justify_end()
        .gap(spacing::STEP_2)
        .px(px_x)
        .py(px_y)
        .border_t_1()
        .border_color(palette.border())
        .bg(palette.muted().opacity(style.footer_wash()))
        .child(footer);
    Some(bar)
}

fn spacer(share: f32) -> Div {
    let mut spacer = div();
    spacer.style().flex_grow = Some(share);
    spacer.style().flex_basis = Some(px(0.0).into());
    spacer
}

fn keyboard(
    layer: Stateful<Div>,
    focus: Focus,
    on_close: Option<CloseHandler>,
) -> Stateful<Div> {
    let (ahead, root) = (focus.clone(), focus.root.clone());
    layer
        .key_context(CONTEXT)
        .track_focus(&root)
        .on_action(move |_: &CloseDialog, window, cx| fire(on_close.as_ref(), window, cx))
        .on_action(move |_: &FocusNext, window, cx| ahead.step(Window::focus_next, window, cx))
        .on_action(move |_: &FocusPrevious, window, cx| {
            focus.step(Window::focus_prev, window, cx);
        })
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

impl RenderOnce for Dialog {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let viewport = window.viewport_size();
        let focus = Focus::kept(self.id.clone(), window, cx);
        let scrim_close = self.on_close.clone();
        let id = self.id.clone();
        let keyed = keyboard(
            div().id((id.clone(), "scrim")),
            focus.clone(),
            self.on_close.clone(),
        );
        let veil = veil::layer(viewport);
        let mut dialog = self;
        dialog.header.close_focus = Some(focus.close);
        let corner = dialog.style.corner();
        let popup = dialog.popup(theme, viewport);
        let layer = keyed
            .occlude()
            .flex()
            .flex_col()
            .items_center()
            .w(viewport.width)
            .h(viewport.height)
            .p(px(VIEWPORT_PADDING))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                fire(scrim_close.as_ref(), window, cx);
            })
            .child(veil)
            .child(spacer(TOP_SHARE))
            .child(crate::Shaped::solid(
                popup.on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation()),
                corner,
            ))
            .child(spacer(BOTTOM_SHARE))
            .with_animation((id, "fade"), fade(cx), Styled::opacity);
        anchored().position(point(px(0.0), px(0.0))).child(layer)
    }
}

opaque_debug!(Dialog);

#[cfg(test)]
mod tests {
    use std::cell::{Cell, RefCell};
    use std::rc::Rc;

    use gpui::{FocusHandle, InteractiveElement, ParentElement, TestAppContext, div, point, px};

    use super::*;
    use crate::floating::Float;
    use crate::test_support::probe::{open, open_floating};

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

    #[gpui::test]
    fn a_body_that_wraps_is_not_cut_off_by_the_footer(cx: &mut TestAppContext) {
        let question =
            "notes.txt has unsaved changes. Do you want to save before closing? ".repeat(2);
        let cx = open(cx, move |_, _| {
            Dialog::new("d")
                .title("Title")
                .size(DialogSize::Sm)
                .body(
                    div()
                        .debug_selector(|| "text".into())
                        .child(question.clone()),
                )
                .footer(div().debug_selector(|| "footer".into()).child("Save"))
        });
        let text = cx.debug_bounds("text").unwrap_or_default();
        let footer = cx.debug_bounds("footer").unwrap_or_default();
        assert!(
            text.bottom() <= footer.top(),
            "the text ends at {:?}, the footer starts at {:?}",
            text.bottom(),
            footer.top()
        );
    }

    type Log = Rc<RefCell<Vec<&'static str>>>;

    fn confirm(log: &Log) -> Dialog {
        let record = |name: &'static str| {
            let log = Rc::clone(log);
            move |_: &gpui::ClickEvent, _: &mut Window, _: &mut App| log.borrow_mut().push(name)
        };
        let closed = Rc::clone(log);
        Dialog::new("d")
            .title("Delete File")
            .body("Are you sure?")
            .footer(
                div()
                    .flex()
                    .child(
                        crate::control::Button::new("cancel", "Cancel").on_click(record("cancel")),
                    )
                    .child(
                        crate::control::Button::new("delete", "Delete").on_click(record("delete")),
                    ),
            )
            .on_close(move |_, _| closed.borrow_mut().push("close"))
    }

    fn open_confirm(cx: &mut TestAppContext) -> (Log, gpui::VisualTestContext) {
        let log = Log::default();
        let shown = Rc::clone(&log);
        let owner = open_floating(cx, move |_, _| {
            let shown = Rc::clone(&shown);
            crate::floating::Portal::window("dialog", move |_, _| confirm(&shown))
                .float(Float::Modal)
        });
        (log, owner)
    }

    fn press(
        cx: &mut gpui::VisualTestContext,
        keys: &'static str,
    ) {
        for key in keys.split(' ') {
            cx.simulate_keystrokes(key);
            let keystroke = gpui::Keystroke::parse(key).unwrap_or_else(|error| panic!("{error}"));
            cx.simulate_event(gpui::KeyUpEvent { keystroke });
        }
    }

    #[gpui::test]
    fn escape_closes_the_dialog(cx: &mut TestAppContext) {
        let (log, mut top) = open_confirm(cx);
        top.simulate_keystrokes("escape");
        assert_eq!(*log.borrow(), vec!["close"]);
    }

    #[gpui::test]
    fn focus_starts_on_the_close_button_where_return_does_nothing(cx: &mut TestAppContext) {
        let (log, mut top) = open_confirm(cx);
        assert!(
            top.update(|window, cx| window.focused(cx).is_some()),
            "the dialog has focus"
        );
        press(&mut top, "enter");
        assert!(log.borrow().is_empty(), "{:?}", log.borrow());
    }

    #[gpui::test]
    fn tab_cycles_close_cancel_delete_and_back_inside_the_dialog(cx: &mut TestAppContext) {
        let (log, mut top) = open_confirm(cx);
        press(&mut top, "tab enter tab enter tab enter tab enter");
        assert_eq!(*log.borrow(), vec!["cancel", "delete", "cancel"]);
        press(&mut top, "shift-tab shift-tab enter");
        assert_eq!(*log.borrow(), vec!["cancel", "delete", "cancel", "delete"]);
    }

    fn counting_dialog(
        field: &FocusHandle,
        typed: &Rc<Cell<usize>>,
    ) -> Dialog {
        let count = Rc::clone(typed);
        let body = div()
            .size(px(20.0))
            .track_focus(field)
            .on_key_down(move |_, _, _| count.set(count.get() + 1));
        Dialog::new("d").title("Rename").body(body)
    }

    #[gpui::test]
    fn a_dialog_body_that_focuses_its_own_field_keeps_that_focus(cx: &mut TestAppContext) {
        let field = cx.update(|cx| cx.focus_handle());
        let (inner, typed) = (field.clone(), Rc::new(Cell::new(0)));
        let count = Rc::clone(&typed);
        let mut owner = open_floating(cx, move |_, _| {
            let (inner, count) = (inner.clone(), Rc::clone(&count));
            crate::floating::Portal::window("dialog", move |_, _| counting_dialog(&inner, &count))
                .float(Float::Modal)
        });
        owner.update(|window, cx| window.focus(&field, cx));
        owner.run_until_parked();
        owner.simulate_keystrokes("a");
        assert_eq!(typed.get(), 1);
    }
}
