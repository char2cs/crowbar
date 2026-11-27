mod header;
mod label_visibility;
mod mark_view;
mod mode_bar;
pub(crate) mod switcher;

pub(crate) use header::host_icon;
pub(crate) use mark_view::mark_view;
pub(crate) use switcher::IconEditor;

use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, Window, div, px};
use sidebar::Mode;
use ui::{Surface, Theme};

use crate::sidebar::FOOT_HEIGHT;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn body(
        &mut self,
        width: f32,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let family = Theme::of(cx).ui_family();
        let shown = self.mode.shown(self.data.focus());
        let panel = match self.create_space_panel(cx) {
            Some(creating) => creating,
            None => match shown {
                Mode::Chats => self.chats_panel(width, window, cx),
                Mode::Files => self.files_panel(window, cx),
                Mode::Changes => self.changes_panel(cx),
            },
        };
        let header = self.header(cx);
        let switcher = self.switcher(window, cx);
        let bar = self.mode_bar(width, cx);
        div()
            .flex()
            .flex_col()
            .relative()
            .w(px(width))
            .h_full()
            .flex_none()
            .overflow_hidden()
            .text_color(palette.foreground())
            .font_family(family)
            .child(header)
            .child(switcher)
            .child(bar)
            .child(div().flex_1().min_h_0().child(panel))
            .child(div().flex_none().h(px(FOOT_HEIGHT)))
            .children(self.toast_layer(cx))
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_column_follows_the_shown_mode(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        for mode in [Mode::Files, Mode::Changes, Mode::Chats] {
            sidebar.update(cx, |this, cx| this.set_mode(mode, cx));
            cx.run_until_parked();
            assert_eq!(sidebar.read_with(cx, |this, _| this.mode), mode);
        }
    }
}
