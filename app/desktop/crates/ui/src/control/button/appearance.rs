//! Builder methods that choose how a button looks.

use gpui::Pixels;

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

    /// Draws the icon at `size` whatever the size step, as an icon with its own size class does.
    #[must_use]
    pub fn icon_size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.icon_size = Some(size);
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
