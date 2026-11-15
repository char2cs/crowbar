//! Everything a [`crate::SettingsModal`] is built from.

use std::rc::Rc;

use gpui::{App, Window};

use crate::close_handler::CloseHandler;
use crate::page_id::PageId;
use crate::primary::Primary;
use crate::record_backend::RecordBackend;
use crate::schema::Schema;
use crate::settings_backend::SettingsBackend;

/// The schema, the two backends, the primary modifier and the optional deep link and close callback of a modal.
pub struct ModalConfig {
    pub(crate) schema: Schema,
    pub(crate) settings: Rc<dyn SettingsBackend>,
    pub(crate) records: Rc<dyn RecordBackend>,
    pub(crate) page: Option<PageId>,
    pub(crate) primary: Primary,
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
            primary: Primary::native(),
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

    /// Labels and captures chords for `primary` instead of the platform default.
    #[must_use]
    pub fn primary(
        mut self,
        primary: Primary,
    ) -> Self {
        self.primary = primary;
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
    use crate::catalog::default_schema;
    use crate::fake_backend::FakeBackend;

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

    #[test]
    fn builders_set_the_page_the_primary_and_the_handler() {
        let config = config()
            .page(PageId::from("git"))
            .primary(Primary::Platform)
            .on_close(|_, _| {});
        assert_eq!(config.page, Some(PageId::from("git")));
        assert_eq!(config.primary, Primary::Platform);
        assert!(config.on_close.is_some());
    }
}
