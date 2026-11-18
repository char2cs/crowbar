//! Builder methods that choose the heading and the width of a dialog.

use gpui::SharedString;

use super::{Dialog, DialogSize, DialogStyle, IconName};

impl Dialog {
    /// Sets the heading.
    #[must_use]
    pub fn title(
        mut self,
        title: impl Into<SharedString>,
    ) -> Self {
        self.header = self.header.title(Some(title.into()));
        self
    }

    /// Shows an icon before the title.
    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.header = self.header.icon(Some(icon));
        self
    }

    /// Picks the React dialog to reproduce: `AppDialog` (default) or the plain `Dialog`.
    #[must_use]
    pub fn style(
        mut self,
        style: DialogStyle,
    ) -> Self {
        self.style = style;
        self.header = self.header.style(style);
        self
    }

    /// Sets the maximum width.
    #[must_use]
    pub fn size(
        mut self,
        size: DialogSize,
    ) -> Self {
        self.size = size;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_builders_choose_style_and_size() {
        let dialog = Dialog::new("d")
            .title("Title")
            .icon(IconName::Plus)
            .style(DialogStyle::Plain)
            .size(DialogSize::Lg);
        assert_eq!(dialog.style, DialogStyle::Plain);
        assert_eq!(dialog.size, DialogSize::Lg);
    }
}
