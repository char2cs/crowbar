//! What the startup view draws: the halt, outage and waiting panels, command buttons, detail boxes, the wordmark and spacing.

mod command_button;
mod detail_box;
mod halt_panel;
mod outage_panel;
mod spacing;
mod waiting_panel;
mod wordmark;

pub(crate) use halt_panel::HaltPanel;
pub(crate) use outage_panel::OutagePanel;
pub(crate) use waiting_panel::WaitingPanel;
pub(crate) use wordmark::wordmark;
