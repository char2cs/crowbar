//! Whether setting rows put the control beside their text or under it.

use gpui::Window;

const STACK_BELOW: f32 = 640.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowLayout {
    Inline,
    Stacked,
}

impl RowLayout {
    pub(crate) fn of(window: &Window) -> Self {
        Self::for_width(f32::from(window.viewport_size().width))
    }

    fn for_width(width: f32) -> Self {
        if width < STACK_BELOW {
            return Self::Stacked;
        }
        Self::Inline
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
