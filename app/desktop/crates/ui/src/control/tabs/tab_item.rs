use gpui::SharedString;

use crate::foundation::IconName;
use crate::foundation::IconWeight;

#[derive(Debug, Clone, PartialEq)]
pub struct TabItem {
    pub(crate) label: Option<SharedString>,
    pub(crate) icon: Option<IconName>,
    pub(crate) selected_weight: IconWeight,
    pub(crate) disabled: bool,
}

impl TabItem {
    pub fn text(label: impl Into<SharedString>) -> Self {
        Self {
            label: Some(label.into()),
            icon: None,
            selected_weight: IconWeight::Regular,
            disabled: false,
        }
    }

    pub fn icon(icon: IconName) -> Self {
        Self {
            label: None,
            icon: Some(icon),
            selected_weight: IconWeight::Fill,
            disabled: false,
        }
    }

    #[must_use]
    pub fn with_icon(
        mut self,
        icon: IconName,
    ) -> Self {
        self.icon = Some(icon);
        self
    }

    #[must_use]
    pub fn with_label(
        mut self,
        label: impl Into<SharedString>,
    ) -> Self {
        self.label = Some(label.into());
        self
    }

    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Expect = (Option<&'static str>, Option<IconName>, IconWeight, bool);

    type Case = (&'static str, fn() -> TabItem, Expect);

    const CASES: &[Case] = &[
        (
            "text tabs have a label and no icon",
            || TabItem::text("Chat"),
            (Some("Chat"), None, IconWeight::Regular, false),
        ),
        (
            "icon tabs fill when selected",
            || TabItem::icon(IconName::Check),
            (None, Some(IconName::Check), IconWeight::Fill, false),
        ),
        (
            "builders add an icon and disable",
            || TabItem::text("Chat").with_icon(IconName::Check).disabled(),
            (
                Some("Chat"),
                Some(IconName::Check),
                IconWeight::Regular,
                true,
            ),
        ),
        (
            "a labelled icon tab keeps its fill weight",
            || TabItem::icon(IconName::Check).with_label("Chats"),
            (
                Some("Chats"),
                Some(IconName::Check),
                IconWeight::Fill,
                false,
            ),
        ),
    ];

    #[test]
    fn constructors_and_builders_set_their_fields() {
        for &(name, build, (label, icon, weight, disabled)) in CASES {
            let tab = build();
            assert_eq!(
                tab.label.as_ref().map(AsRef::as_ref),
                label,
                "{name}: label"
            );
            assert_eq!(tab.icon, icon, "{name}: icon");
            assert_eq!(tab.selected_weight, weight, "{name}: weight");
            assert_eq!(tab.disabled, disabled, "{name}: disabled");
        }
    }
}
