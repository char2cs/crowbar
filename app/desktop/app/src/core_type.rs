use panehost::{Multiplicity, PaneKind, PaneLifecycle};

pub(crate) struct CoreType {
    pub(crate) id: &'static str,
    pub(crate) title: &'static str,
    pub(crate) entry: &'static str,
    pub(crate) kind: PaneKind,
    pub(crate) lifecycle: PaneLifecycle,
    pub(crate) multiplicity: Multiplicity,
}
