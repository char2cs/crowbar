use gpui::Window;
use ui::Breakpoint;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowLayout {
    Inline,
    Stacked,
}

impl RowLayout {
    pub(crate) fn of(window: &Window) -> Self {
        Self::for_width(ui::app_window_width(window))
    }

    fn for_width(width: f32) -> Self {
        if Breakpoint::cleared_by(width).contains(&Breakpoint::Sm) {
            return Self::Inline;
        }
        Self::Stacked
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_stack_below_640px_as_develops_max_640_query() {
        let cases = [
            (639.0, RowLayout::Stacked),
            (640.0, RowLayout::Inline),
            (1280.0, RowLayout::Inline),
        ];
        for (width, expected) in cases {
            assert_eq!(RowLayout::for_width(width), expected, "{width}");
        }
    }
}
