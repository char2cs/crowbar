//! A pane's life: runtime state, phases, stages, frames, visibility and slot placement.

mod frames;
pub(crate) mod pane_phase;
pub(crate) mod pane_runtime;
mod placement;
mod stage;
mod teardown;
mod visibility;
