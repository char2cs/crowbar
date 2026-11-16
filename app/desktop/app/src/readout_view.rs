use gpui::{Context, IntoElement, ParentElement as _, Render, Styled as _, Window, div, px};
use ui::{Surface, Theme};

use crate::readout_dock::ReadoutDock;
use crate::readout_lines::ReadoutLines;

const TEXT_SIZE: f32 = 10.0;
const FOOT_HEIGHT: f32 = 40.0;
const INSET: f32 = 12.0;
const TOP_WIDTH: f32 = 340.0;
const WARNING_OPACITY: f32 = 1.0;
const QUIET_OPACITY: f32 = 0.7;

/// The build readout: build, link and frame lines that take no pointer events.
#[derive(Debug, Default)]
pub(crate) struct ReadoutView {
    lines: ReadoutLines,
    dock: ReadoutDock,
}

impl ReadoutView {
    pub(crate) fn show(
        &mut self,
        (lines, dock): (ReadoutLines, ReadoutDock),
        cx: &mut Context<'_, Self>,
    ) {
        self.lines = lines;
        self.dock = dock;
        cx.notify();
    }
}

impl Render for ReadoutView {
    fn render(
        &mut self,
        _window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let theme = Theme::of(cx);
        let palette = theme.palette(Surface::Chrome);
        let color = if self.lines.warns {
            palette.git_deleted()
        } else {
            palette.muted_foreground()
        };
        let opacity = if self.lines.warns {
            WARNING_OPACITY
        } else {
            QUIET_OPACITY
        };
        let text = div()
            .absolute()
            .flex()
            .flex_col()
            .items_center()
            .justify_end()
            .text_size(px(TEXT_SIZE))
            .font_family(theme.mono_family())
            .text_color(color)
            .opacity(opacity)
            .child(self.lines.build.clone())
            .child(self.lines.net.clone())
            .child(self.lines.stats.clone());
        match self.dock {
            ReadoutDock::Left(width) => text
                .left(px(INSET))
                .bottom_0()
                .w(px(width - 2.0 * INSET))
                .h(px(FOOT_HEIGHT)),
            ReadoutDock::Right(width) => text
                .right(px(INSET))
                .bottom_0()
                .w(px(width - 2.0 * INSET))
                .h(px(FOOT_HEIGHT)),
            ReadoutDock::Top => text.top_0().left_0().right_0().mx_auto().w(px(TOP_WIDTH)),
        }
    }
}
