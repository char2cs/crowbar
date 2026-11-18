//! A modal in a test window over a [`FakeBackend`].

use std::cell::Cell;
use std::rc::Rc;

use gpui::{Entity, TestAppContext, VisualTestContext};
use settings::{
    AGENT_RECORDS, DESCRIPTOR_RECORDS, FakeBackend, HOST_RECORDS, PageId, Record, RecordFlag,
    Records, Row, SESSION_RECORDS, Snapshot, Values, default_schema,
};

use crate::intent::Intent;
use crate::intent::Runner as _;
use crate::settings_modal::ModalConfig;
use crate::settings_modal::SettingsModal;

pub(crate) struct Harness {
    pub(crate) modal: Entity<SettingsModal>,
    pub(crate) backend: FakeBackend,
    pub(crate) closed: Rc<Cell<u32>>,
}

pub(crate) fn backend() -> FakeBackend {
    let agent = Record::new("claude", "Claude Code")
        .flag(RecordFlag::new("enabled", "Enabled").on())
        .movable();
    let second = Record::new("codex", "Codex")
        .flag(RecordFlag::new("enabled", "Enabled"))
        .movable();
    FakeBackend::new(Snapshot::new(Values::default()).with_capability("browser.clearData"))
        .with_records(AGENT_RECORDS, Records::Ready(vec![agent, second]))
        .with_records(DESCRIPTOR_RECORDS, Records::Loading)
        .with_records(SESSION_RECORDS, Records::Failed)
        .with_records(HOST_RECORDS, Records::Ready(Vec::new()))
}

pub(crate) fn open<'a>(
    cx: &'a mut TestAppContext,
    page: &str,
) -> (Harness, &'a mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).unwrap());
    let backend = backend();
    let closed = Rc::new(Cell::new(0));
    let counter = Rc::clone(&closed);
    let config = ModalConfig::new(
        default_schema().unwrap(),
        Rc::new(backend.clone()),
        Rc::new(backend.clone()),
    )
    .page(PageId::from(page))
    .on_close(move |_, _| counter.set(counter.get() + 1));
    let (modal, cx) = cx.add_window_view(|window, cx| SettingsModal::new(config, window, cx));
    cx.run_until_parked();
    (
        Harness {
            modal,
            backend,
            closed,
        },
        cx,
    )
}

pub(crate) fn row(key: &str) -> Row {
    default_schema()
        .unwrap()
        .rows()
        .find(|row| row.key.as_str() == key)
        .cloned()
        .unwrap()
}

pub(crate) fn run(
    harness: &Harness,
    intent: Intent,
    cx: &mut VisualTestContext,
) {
    harness
        .modal
        .update_in(cx, |modal, window, cx| modal.run(intent, window, cx));
    cx.run_until_parked();
}

pub(crate) fn inspect<T>(
    harness: &Harness,
    cx: &mut VisualTestContext,
    read: impl FnOnce(&SettingsModal) -> T,
) -> T {
    harness.modal.read_with(cx, |modal, _| read(modal))
}
