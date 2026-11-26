//! The IDE shell model, after develop's `PaneGroup`: each place of a tab's split tree holds a
//! chat and the editor tabs beside it, with develop's tab-bar commands (pin, bulk close, keyboard
//! steps, move between groups).

pub(crate) mod command;
pub(crate) mod pane_group;
pub(crate) mod presentation;
mod tab;
