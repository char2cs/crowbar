use gpui::{IntoElement, SharedString};

use super::{Dialog, DialogSize, DialogStyle, HeaderFlow, IconName};

impl Dialog {
    #[must_use]
    pub fn title(
        mut self,
        title: impl Into<SharedString>,
    ) -> Self {
        self.header = self.header.title(Some(title.into()));
        self
    }

    #[must_use]
    pub fn heading(
        mut self,
        heading: impl IntoElement,
    ) -> Self {
        self.header = self.header.heading(Some(heading.into_any_element()));
        self
    }

    #[must_use]
    pub fn header_flow(
        mut self,
        flow: HeaderFlow,
    ) -> Self {
        self.header = self.header.flow(flow);
        self
    }

    #[must_use]
    pub fn icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.header = self.header.icon(Some(icon));
        self
    }

    #[must_use]
    pub fn style(
        mut self,
        style: DialogStyle,
    ) -> Self {
        self.style = style;
        self.header = self.header.style(style);
        self
    }

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
