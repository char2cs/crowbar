use gpui::{Pixels, Stateful, Styled, px};

use crate::foundation::Palette;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum DialogStyle {
    #[default]
    App,
    Plain,
}

impl DialogStyle {
    pub fn title_size(self) -> Pixels {
        match self {
            Self::App => text_size::SM,
            Self::Plain => text_size::XL,
        }
    }

    pub fn title_line(self) -> Pixels {
        match self {
            Self::App => spacing::STEP_3_5,
            Self::Plain => text_size::XL,
        }
    }

    pub fn corner(self) -> Pixels {
        match self {
            Self::App => radius::XL,
            Self::Plain => radius::XXL,
        }
    }

    pub fn header_padding(self) -> (Pixels, Pixels) {
        match self {
            Self::App => (spacing::STEP_4, spacing::STEP_3),
            Self::Plain => (spacing::STEP_6, spacing::STEP_6),
        }
    }

    pub fn footer_padding(self) -> (Pixels, Pixels) {
        match self {
            Self::App => (spacing::STEP_4, spacing::STEP_3),
            Self::Plain => (spacing::STEP_6, spacing::STEP_4),
        }
    }

    pub fn footer_wash(self) -> f32 {
        match self {
            Self::App => 0.5,
            Self::Plain => opacity::PERCENT_72,
        }
    }

    pub fn frame(
        self,
        frame: Stateful<gpui::Div>,
        palette: &Palette,
    ) -> Stateful<gpui::Div> {
        let frame = frame.rounded(self.corner());
        match self {
            Self::App => frame,
            Self::Plain => frame.border_1().border_color(palette.border()),
        }
    }

    pub fn body_padding(self) -> Pixels {
        match self {
            Self::App => spacing::STEP_4,
            Self::Plain => px(0.0),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (DialogStyle, f32, f32, f32, f32);

    #[test]
    fn each_style_has_the_metrics_of_its_react_component() {
        const CASES: &[Case] = &[
            (DialogStyle::App, 14.0, 14.0, 14.0, 16.0),
            (DialogStyle::Plain, 20.0, 20.0, 18.0, 0.0),
        ];
        for &(style, size, line, corner, body) in CASES {
            assert_eq!(f32::from(style.title_size()), size, "{style:?} size");
            assert_eq!(f32::from(style.title_line()), line, "{style:?} line");
            assert_eq!(f32::from(style.corner()), corner, "{style:?} corner");
            assert_eq!(f32::from(style.body_padding()), body, "{style:?} body");
        }
    }

    #[test]
    fn only_the_plain_footer_is_washed_at_72_percent() {
        assert!((DialogStyle::App.footer_wash() - 0.5).abs() < f32::EPSILON);
        assert!((DialogStyle::Plain.footer_wash() - 0.72).abs() < f32::EPSILON);
    }

    #[test]
    fn plain_chrome_is_roomier_than_the_app_chrome() {
        let (app, plain) = (DialogStyle::App, DialogStyle::Plain);
        assert!(plain.header_padding().1 > app.header_padding().1);
        assert!(plain.footer_padding().0 > app.footer_padding().0);
    }
}
