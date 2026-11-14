//! Everything a [`crate::SettingsModal`] is built from.

use std::rc::Rc;

use gpui::{App, Window};

use crate::close_handler::CloseHandler;
use crate::page_id::PageId;
use crate::record_backend::RecordBackend;
use crate::schema::Schema;
use crate::settings_backend::SettingsBackend;

/// The schema, the two backends and the optional deep link and close callback of a modal.
pub struct ModalConfig {
    pub(crate) schema: Schema,
    pub(crate) settings: Rc<dyn SettingsBackend>,
    pub(crate) records: Rc<dyn RecordBackend>,
    pub(crate) page: Option<PageId>,
    pub(crate) on_close: Option<CloseHandler>,
}

impl ModalConfig {
    /// A configuration that shows `schema` over `settings` and `records`.
    pub fn new(
        schema: Schema,
        settings: Rc<dyn SettingsBackend>,
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
    pub fn page(mut self, page: PageId) -> Self {
        self.page = Some(page);
        self
    }

    /// Calls `handler` when the user closes the modal (close button, scrim, Escape).
    #[must_use]
    pub fn on_close(mut self, handler: impl Fn(&mut Window, &mut App) + 'static) -> Self {
        self.on_close = Some(Rc::new(handler));
        self
    }
}

impl std::fmt::Debug for ModalConfig {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ModalConfig")
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
#[path = "modal_config_tests.rs"]
mod tests;
