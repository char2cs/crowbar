//! One entry of a [`crate::Tabs`] list.

use gpui::SharedString;

use crate::foundation::IconName;
use crate::foundation::IconWeight;

/// A tab: label, optional icon and the weight the icon takes while selected.
#[derive(Debug, Clone, PartialEq)]
pub struct TabItem {
    pub(crate) label: Option<SharedString>,
    pub(crate) icon: Option<IconName>,
    pub(crate) selected_weight: IconWeight,
    pub(crate) disabled: bool,
}

impl TabItem {
    /// A text tab.
    pub fn text(label: impl Into<SharedString>) -> Self {
        Self {
            label: Some(label.into()),
            icon: None,
            selected_weight: IconWeight::Regular,
            disabled: false,
        }
    }

    /// An icon-only tab; the icon turns `Fill` while selected like the sidebar mode switch.
    pub fn icon(icon: IconName) -> Self {
        Self {
            label: None,
            icon: Some(icon),
            selected_weight: IconWeight::Fill,
            disabled: false,
        }
    }

    /// Adds a text label next to the icon; the selected weight is kept.
    #[must_use]
    pub fn with_label(
        mut self,
        label: impl Into<SharedString>,
    ) -> Self {
        self.label = Some(label.into());
        self
    }

    /// Marks the tab as not selectable.
    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }
}
