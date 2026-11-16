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
const SEGMENT_GAP: &str = "  ";

#[derive(Debug, Default)]
pub(crate) struct ReadoutView {
    lines: ReadoutLines,
    dock: ReadoutDock,
}

fn opacity_of(lines: &ReadoutLines) -> f32 {
    if lines.warns {
        return WARNING_OPACITY;
    }
    QUIET_OPACITY
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
        let mut color = palette.muted_foreground();
        if self.lines.warns {
            color = palette.git_deleted();
        }
        let text = div()
            .absolute()
            .flex()
            .flex_col()
            .items_start()
            .justify_end()
            .text_size(px(TEXT_SIZE))
            .font_family(theme.mono_family())
            .text_color(color)
            .opacity(opacity_of(&self.lines))
            .child(format!(
                "{}{SEGMENT_GAP}{}",
                self.lines.build, self.lines.net
            ))
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

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    fn warning() -> ReadoutLines {
        ReadoutLines {
            warns: true,
            ..ReadoutLines::default()
        }
    }

    #[test]
    fn a_warning_reads_at_full_strength_and_a_quiet_readout_dims() {
        let cases = [
            (warning(), WARNING_OPACITY),
            (ReadoutLines::default(), QUIET_OPACITY),
        ];
        for (lines, opacity) in cases {
            assert!(
                (opacity_of(&lines) - opacity).abs() < f32::EPSILON,
                "{lines:?}"
            );
        }
    }

    #[gpui::test]
    fn showing_replaces_the_lines_and_the_dock(cx: &mut TestAppContext) {
        let view = cx.new(|_| ReadoutView::default());
        let lines = ReadoutLines {
            net: "net DOWN".into(),
            warns: true,
            ..ReadoutLines::default()
        };
        view.update(cx, |view, cx| {
            view.show((lines.clone(), ReadoutDock::Right(300.0)), cx);
        });
        let shown = view.read_with(cx, |view, _| (view.lines.clone(), view.dock));
        assert_eq!(shown, (lines, ReadoutDock::Right(300.0)));
    }
}
