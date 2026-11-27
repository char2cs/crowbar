use gpui::Hsla;
use ui::Palette;
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RowInk {
    Muted,
    Strong,
}

impl RowInk {
    pub(crate) fn color(
        self,
        palette: &Palette,
    ) -> Hsla {
        match self {
            Self::Muted => palette.muted_foreground(),
            Self::Strong => palette.foreground(),
        }
    }
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn a_pressed_toggle_inks_stronger_than_a_resting_button() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Window);
        assert_eq!(RowInk::Muted.color(palette), palette.muted_foreground());
        assert_eq!(RowInk::Strong.color(palette), palette.foreground());
    }
}
