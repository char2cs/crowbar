use std::cell::{Cell, RefCell};
use std::rc::Rc;

use gpui::{
    Context, Image, ImageFormat, InteractiveElement as _, IntoElement, Modifiers, MouseExitEvent,
    ParentElement as _, Render, StatefulInteractiveElement as _, Styled as _, StyledText,
    TestAppContext, TextLayout, Window, div, point, px,
};

#[cfg(target_os = "linux")]
#[path = "support/on_x11.rs"]
mod needs_display;

const RED_SVG: &str = r##"<svg xmlns="http://www.w3.org/2000/svg" width="2" height="2"><rect width="2" height="2" fill="#ff0000"/></svg>"##;
const BGRA_RED: [u8; 4] = [0, 0, 255, 255];
const LONG_NAME: &str = "a-file-name-far-too-long-for-its-narrow-row.rs";
const ELLIPSIS: char = '…';
const ROW_WIDTH: f32 = 120.0;
const PROBE_SIDE: f32 = 40.0;
const INSIDE: f32 = 10.0;
#[gpui::test]
fn a_red_svg_image_decodes_to_red_in_bgra_order(cx: &mut TestAppContext) {
    let image = Image::from_bytes(ImageFormat::Svg, RED_SVG.as_bytes().to_vec());
    let decoded = cx
        .update(|cx| image.to_image_data(cx.svg_renderer()))
        .expect("the svg decodes");
    let first = decoded.as_bytes(0).map(|bytes| bytes[..4].to_vec());
    assert_eq!(first, Some(BGRA_RED.to_vec()), "first pixel of a red svg");
}

struct TruncatedRow(Rc<RefCell<Option<TextLayout>>>);

impl Render for TruncatedRow {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let text = StyledText::new(LONG_NAME);
        *self.0.borrow_mut() = Some(text.layout().clone());
        div()
            .flex()
            .w(px(ROW_WIDTH))
            .child(div().truncate().child(text))
    }
}

#[gpui::test]
fn a_truncated_flex_item_ends_in_an_ellipsis(cx: &mut TestAppContext) {
    let layout = Rc::new(RefCell::new(None));
    let (_, cx) = cx.add_window_view(|_, _| TruncatedRow(Rc::clone(&layout)));
    cx.run_until_parked();
    let shown = layout.borrow().as_ref().map(TextLayout::text);
    assert!(
        shown
            .as_deref()
            .is_some_and(|text| text.ends_with(ELLIPSIS)),
        "{shown:?}"
    );
}

struct HoverProbe(Rc<Cell<bool>>);

impl Render for HoverProbe {
    fn render(
        &mut self,
        _: &mut Window,
        _: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let seen = Rc::clone(&self.0);
        div()
            .id("probe")
            .size(px(PROBE_SIDE))
            .on_hover(move |hovered, _, _| seen.set(*hovered))
    }
}

#[gpui::test]
fn leaving_the_window_ends_the_hover(cx: &mut TestAppContext) {
    let hovered = Rc::new(Cell::new(false));
    let (_, cx) = cx.add_window_view(|_, _| HoverProbe(Rc::clone(&hovered)));
    let inside = point(px(INSIDE), px(INSIDE));
    cx.simulate_mouse_move(inside, None, Modifiers::none());
    assert!(hovered.get(), "the pointer over the probe hovers it");
    cx.simulate_event(MouseExitEvent {
        position: inside,
        pressed_button: None,
        modifiers: Modifiers::none(),
    });
    assert!(!hovered.get(), "the pointer left the window");
}
