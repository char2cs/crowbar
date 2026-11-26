//! Everything a [`crate::SettingsModal`] is built from.

use std::rc::Rc;

use gpui::{App, Window};
use settings::{PageId, RecordBackend, Schema};

use crate::settings_modal::close_handler::CloseHandler;

/// The schema, the two backends and the optional deep link and close callback of a modal.
pub struct ModalConfig {
    pub(crate) schema: Schema,
    pub(crate) settings: Rc<dyn settings::Backend>,
    pub(crate) records: Rc<dyn RecordBackend>,
    pub(crate) page: Option<PageId>,
    pub(crate) on_close: Option<CloseHandler>,
}

impl ModalConfig {
    /// A configuration that shows `schema` over `settings` and `records`.
    pub fn new(
        schema: Schema,
        settings: Rc<dyn settings::Backend>,
        records: Rc<dyn RecordBackend>,
    ) -> Self {
        Self {
            schema,
            settings,
            records,
            page: None,
            on_close: None,
        }
    }

    /// Opens on `page` instead of the first page.
    #[must_use]
    pub fn page(
        mut self,
        page: PageId,
    ) -> Self {
        self.page = Some(page);
        self
    }

    /// Calls `handler` when the user closes the modal (close button, scrim, Escape).
    #[must_use]
    pub fn on_close(
        mut self,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_close = Some(Rc::new(handler));
        self
    }
}

impl std::fmt::Debug for ModalConfig {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("ModalConfig")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use super::*;
    use settings::{FakeBackend, default_schema};

    fn config() -> ModalConfig {
        let backend = FakeBackend::default();
        ModalConfig::new(
            default_schema().unwrap(),
            Rc::new(backend.clone()),
            Rc::new(backend),
        )
    }

    #[test]
    fn starts_without_a_deep_link_or_close_handler() {
        let config = config();
        assert!(config.page.is_none() && config.on_close.is_none());
    }
}
