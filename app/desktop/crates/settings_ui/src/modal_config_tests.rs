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
fn builders_set_the_page_and_the_handler() {
    let config = config().page(PageId::from("git")).on_close(|_, _| {});
    assert_eq!(config.page, Some(PageId::from("git")));
    assert!(config.on_close.is_some());
}
