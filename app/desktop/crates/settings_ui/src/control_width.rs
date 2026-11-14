//! Width classes of the controls in a row (`SETTINGS_CONTROL_WIDTHS`).

/// How wide a row's control is.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ControlWidth {
    /// 112px.
    Compact,
    /// 144px.
    Default,
    /// 176px.
    Wide,
    /// 224px.
    ExtraWide,
    /// 96px number stepper.
    NumberCompact,
    /// 192px text field.
    Text,
    /// 224px text field.
    TextWide,
}

impl ControlWidth {
    /// Width in pixels at UI scale 1.
    pub fn pixels(self) -> f32 {
        match self {
            Self::NumberCompact => 96.0,
            Self::Compact => 112.0,
            Self::Default => 144.0,
            Self::Wide => 176.0,
            Self::Text => 192.0,
            Self::ExtraWide | Self::TextWide => 224.0,
        }
    }
}

#[cfg(test)]
#[path = "control_width_tests.rs"]
mod tests;
