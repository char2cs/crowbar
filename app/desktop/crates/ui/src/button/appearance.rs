//! Builder methods that choose how a button looks.

use super::{Button, ButtonSize, ButtonVariant, IconName, Surface};

impl Button {
    /// Adds a leading icon to a labelled button.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Chooses the colour variant.
    #[must_use]
    pub fn variant(
        mut self,
        variant: ButtonVariant,
    ) -> Self {
        self.variant = variant;
        self
    }

    /// Chooses the size step.
    #[must_use]
    pub fn size(
        mut self,
        size: ButtonSize,
    ) -> Self {
        self.size = size;
        self
    }

    /// Paints the button with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_builders_choose_icon_variant_size_and_surface() {
        let button = Button::new("b", "Save")
            .icon(IconName::Plus)
            .variant(ButtonVariant::Ghost)
            .size(ButtonSize::Sm)
            .on(Surface::Window);
        assert_eq!(button.icon, Some(IconName::Plus));
        assert_eq!(button.variant, ButtonVariant::Ghost);
        assert_eq!(button.size, ButtonSize::Sm);
        assert_eq!(button.surface, Surface::Window);
    }
}
