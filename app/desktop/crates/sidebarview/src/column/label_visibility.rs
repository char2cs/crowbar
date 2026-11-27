const ACTIVE_LABEL_FROM: f32 = 280.0;
const ALL_LABELS_FROM: f32 = 420.0;
const BAR_PADDING: f32 = 16.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum LabelVisibility {
    Hidden,
    ActiveOnly,
    All,
}

impl LabelVisibility {
    pub(crate) fn for_sidebar(sidebar_width: f32) -> Self {
        let bar = sidebar_width - BAR_PADDING;
        if bar >= ALL_LABELS_FROM {
            return Self::All;
        }
        if bar >= ACTIVE_LABEL_FROM {
            return Self::ActiveOnly;
        }
        Self::Hidden
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn thresholds_apply_to_the_bar_width() {
        let cases = [
            (250.0, LabelVisibility::Hidden),
            (294.0, LabelVisibility::Hidden),
            (296.0, LabelVisibility::ActiveOnly),
            (435.0, LabelVisibility::ActiveOnly),
            (436.0, LabelVisibility::All),
            (640.0, LabelVisibility::All),
        ];
        for (width, expected) in cases {
            assert_eq!(
                LabelVisibility::for_sidebar(width),
                expected,
                "width {width}"
            );
        }
    }
}
