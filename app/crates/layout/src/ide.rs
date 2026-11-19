//! The IDE shell model, after develop's `PaneGroup`: each place of a tab's split tree holds a
//! chat and the editor tabs beside it, with develop's tab-bar commands (pin, bulk close, keyboard
//! steps, move between groups).

pub(crate) mod close_panes;
pub(crate) mod close_scope;
pub(crate) mod group_commands;
mod group_edit;
pub(crate) mod pane_group;
pub(crate) mod pin_commands;
mod pins;
pub(crate) mod presentation;
pub(crate) mod sidebar_side;
pub(crate) mod strip_step;
mod tab_chat;
mod tab_leaves;
mod tab_seat;
pub(crate) mod view_mode;
