//! One entry of a [`crate::Tabs`] list.

use gpui::SharedString;

use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;

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
        Self { label: Some(label.into()), icon: None, selected_weight: IconWeight::Regular, disabled: false }
    }

    /// An icon-only tab; the icon turns `Fill` while selected like the sidebar mode switch.
    pub fn icon(icon: IconName) -> Self {
        Self { label: None, icon: Some(icon), selected_weight: IconWeight::Fill, disabled: false }
    }

    /// Adds an icon next to the label.
    #[must_use]
    pub fn with_icon(mut self, icon: IconName) -> Self {
        self.icon = Some(icon);
        self
    }

    /// Marks the tab as not selectable.
    #[must_use]
    pub fn disabled(mut self, disabled: bool) -> Self {
        self.disabled = disabled;
        self
    }
}
