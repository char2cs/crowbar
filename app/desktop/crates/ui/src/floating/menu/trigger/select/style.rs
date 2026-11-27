//! Builder methods that size and label a select.

use gpui::{Pixels, SharedString, px};

use super::{COMPACT_HEIGHT, Select};

impl Select {
    /// Text shown while nothing is selected.
    #[must_use]
    pub fn placeholder(
        mut self,
        placeholder: impl Into<SharedString>,
    ) -> Self {
        self.placeholder = placeholder.into();
        self
    }

    /// Sets the width of the trigger.
    #[must_use]
    pub fn width(
        mut self,
        width: Pixels,
    ) -> Self {
        self.width = width;
        self
    }

    /// Stretches the trigger to the width of its parent.
    #[must_use]
    pub fn fill(mut self) -> Self {
        self.fills = true;
        self
    }

    /// Makes the trigger 28px tall, as in a row of settings.
    #[must_use]
    pub fn compact(mut self) -> Self {
        self.height = px(COMPACT_HEIGHT);
        self
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::probe::measure;

    fn options() -> Vec<SharedString> {
        vec!["System".into(), "Light".into(), "Dark".into()]
    }

    #[gpui::test]
    fn trigger_is_thirty_two_pixels_high_and_as_wide_as_requested(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            Select::new("s", options(), Some(1)).width(px(160.0))
        });
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (160.0, 32.0)
        );
    }

    #[gpui::test]
    fn compact_triggers_are_28px_tall(cx: &mut TestAppContext) {
        let compact = measure(cx, |_, _| {
            Select::new("s", vec!["A".into()], Some(0)).compact()
        });
        let standard = measure(cx, |_, _| Select::new("s", vec!["A".into()], Some(0)));
        assert_eq!(
            (
                f32::from(compact.size.height),
                f32::from(standard.size.height)
            ),
            (28.0, 32.0)
        );
    }
}
