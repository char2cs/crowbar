const COMPACT_HEIGHT: f32 = 20.0;
const DEFAULT_HEIGHT: f32 = 26.0;
const COMFORTABLE_HEIGHT: f32 = 30.0;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum FileDensity {
    Compact,
    #[default]
    Standard,
    Comfortable,
}

impl FileDensity {
    pub fn from_setting(text: &str) -> Self {
        match text {
            "compact" => Self::Compact,
            "comfortable" => Self::Comfortable,
            _ => Self::Standard,
        }
    }

    pub fn row_height(self) -> f32 {
        match self {
            Self::Compact => COMPACT_HEIGHT,
            Self::Standard => DEFAULT_HEIGHT,
            Self::Comfortable => COMFORTABLE_HEIGHT,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_setting_picks_its_density_and_unknown_text_the_standard_one() {
        let cases = [
            ("compact", FileDensity::Compact, 20.0),
            ("default", FileDensity::Standard, 26.0),
            ("comfortable", FileDensity::Comfortable, 30.0),
            ("roomy", FileDensity::Standard, 26.0),
        ];
        for (text, density, height) in cases {
            let parsed = FileDensity::from_setting(text);
            assert_eq!(parsed, density, "{text}");
            assert!(
                (parsed.row_height() - height).abs() < f32::EPSILON,
                "{text}"
            );
        }
    }
}
