use gpui::{AnyElement, App, IntoElement, ParentElement as _, Styled as _, div, px};
use ui::Palette;

use super::{COUNT_MARGIN, FindWidget, LABEL_SIZE};

// SPEC: https://github.com/microsoft/vscode/blob/main/src/vs/editor/contrib/find/browser/findWidget.css
const COUNT_MIN_WIDTH: f32 = 69.0;
const NO_RESULTS: &str = "No results";

impl FindWidget {
    pub(crate) fn label(
        &self,
        cx: &App,
    ) -> String {
        count_label(self.count(cx), self.current)
    }

    pub(super) fn count_element(
        &self,
        palette: &Palette,
        cx: &App,
    ) -> AnyElement {
        let ink = match self.count(cx) {
            Some(0) => palette.destructive(),
            _ => palette.foreground(),
        };
        div()
            .flex_none()
            .min_w(px(COUNT_MIN_WIDTH))
            .ml(px(COUNT_MARGIN))
            .text_size(px(LABEL_SIZE))
            .text_color(ink)
            .child(self.label(cx))
            .into_any_element()
    }

    fn count(
        &self,
        cx: &App,
    ) -> Option<usize> {
        let query_empty = self.find_input.read(cx).text().is_empty();
        (!query_empty).then_some(self.matches.len())
    }
}

fn count_label(
    count: Option<usize>,
    current: Option<usize>,
) -> String {
    match (count, current) {
        (None | Some(0), _) => NO_RESULTS.to_owned(),
        (Some(count), Some(current)) => format!("{} of {count}", current + 1),
        (Some(count), None) => format!("? of {count}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_count_reads_as_monaco_s() {
        let cases = [
            (None, None, "No results"),
            (Some(0), None, "No results"),
            (Some(7), Some(2), "3 of 7"),
            (Some(7), None, "? of 7"),
        ];
        for (count, current, expected) in cases {
            assert_eq!(
                count_label(count, current),
                expected,
                "{count:?} {current:?}"
            );
        }
    }
}
