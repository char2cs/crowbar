use pane_host::{Multiplicity, PaneKind, PaneLifecycle};

pub(crate) struct CoreType {
    pub(crate) id: &'static str,
    pub(crate) title: &'static str,
    // SPEC: an empty entry means no bundle: the browser loads the user's page and
    // SPEC: a native pane is painted by the shell.
    pub(crate) entry: &'static str,
    pub(crate) kind: PaneKind,
    pub(crate) lifecycle: PaneLifecycle,
    pub(crate) multiplicity: Multiplicity,
}
