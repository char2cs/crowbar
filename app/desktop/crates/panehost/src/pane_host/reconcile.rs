//! Sync input and output: the layout snapshot, visible panes, what the shell's overlays cover
//! and cut out,
//! keyboard owner and where native focus goes, sync report, failures and pane display.

pub(crate) mod covered;
pub(crate) mod failures;
pub(crate) mod keyboard;
pub(crate) mod layout_snapshot;
mod native_focus;
mod overlays;
pub(crate) mod pane_display;
pub(crate) mod pane_kinds;
pub(crate) mod sync_report;
mod visible_pane;

pub use covered::Covered;
pub(crate) use failures::Failures;
pub use keyboard::Keyboard;
pub use layout_snapshot::LayoutSnapshot;
pub use overlays::Overlays;
pub use pane_display::PaneDisplay;
pub use pane_kinds::PaneKinds;
pub use sync_report::SyncReport;
pub use visible_pane::VisiblePane;
