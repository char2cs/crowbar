//! The sidebar column: header, switcher, mode bar, the shown panel and the foot spacer.

use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{Surface, Theme};

use crate::metrics::FOOT_HEIGHT;
use crate::mode::Mode;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn body(
        &mut self,
        width: f32,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let shown = self.mode.shown(self.data.focus());
        let panel = match shown {
            Mode::Chats => self.chats_panel(width, window, cx),
            Mode::Files => self.files_panel(cx),
            Mode::Changes => self.changes_panel(cx),
        };
        let header = self.header(cx);
        let switcher = self.switcher(window, cx);
        let bar = self.mode_bar(width, cx);
        div()
            .flex()
            .flex_col()
            .w(px(width))
            .h_full()
            .flex_none()
            .overflow_hidden()
            .text_color(palette.foreground())
            .font_family(ui::font::UI_FAMILY)
            .child(header)
            .children(switcher)
            .child(bar)
            .child(div().flex_1().min_h_0().child(panel))
            .child(div().flex_none().h(px(FOOT_HEIGHT)))
            .into_any_element()
    }
}

#[cfg(test)]
#[path = "body_tests.rs"]
mod tests;
