use gpui::{Div, Entity, ParentElement, SharedString, Styled, div, px};
use ui::{Checkbox, Surface};

use crate::scale::{SPACE_MD, TEXT_SMALL};
use crate::sidebar::Sidebar;

const LINE_HEIGHT: f32 = 16.0;

pub(crate) fn path_row(
    path: &str,
    ticked: &[String],
    sidebar: &Entity<Sidebar>,
) -> Div {
    let (target, owner) = (path.to_owned(), sidebar.clone());
    let id = SharedString::from(format!("merge-path-{path}"));
    let checkbox = Checkbox::new(id, ticked.iter().any(|candidate| candidate == path))
        .on(Surface::Chrome)
        .on_change(move |_, _, cx| {
            owner.update(cx, |this, cx| this.toggle_path(&target, cx));
        });
    div()
        .flex()
        .items_center()
        .gap(SPACE_MD)
        .text_size(TEXT_SMALL)
        .line_height(px(LINE_HEIGHT))
        .child(checkbox)
        .child(div().truncate().child(path.to_owned()))
}

pub(crate) fn note(
    text: String,
    color: gpui::Hsla,
) -> Div {
    div()
        .text_size(TEXT_SMALL)
        .line_height(px(LINE_HEIGHT))
        .text_color(color)
        .child(text)
}

#[cfg(test)]
mod tests {
    use gpui::hsla;

    use super::*;

    #[test]
    fn a_note_is_drawn_in_the_colour_it_was_given() {
        let color = hsla(0.5, 0.5, 0.5, 1.0);
        let mut line = note("careful".to_owned(), color);
        assert_eq!(line.text_style().as_ref().and_then(|style| style.color), Some(color));
    }
}
