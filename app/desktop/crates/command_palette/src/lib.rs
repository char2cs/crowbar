//! The command palette of the Crowbar shell.
//!
//! [`rank`] is the pure part: it filters [`PaletteEntry`]s by a typed query and orders them by
//! how well [`fuzzy_match`] fits. [`PalettePanel`] is the GPUI part: a dialog with a search field
//! and the ranked list, which emits [`PaletteEvent`] when the user picks an entry or closes it.

mod command_palette;
mod emphasis;
mod fuzzy_match;
mod palette_entry;
mod palette_event;
mod palette_panel;
mod palette_row;
mod ranked_entry;
mod row_state;
mod step_direction;

pub use command_palette::rank;
pub use fuzzy_match::{FuzzyMatch, fuzzy_match};
pub use palette_entry::PaletteEntry;
pub use palette_event::PaletteEvent;
pub use palette_panel::PalettePanel;
pub use ranked_entry::RankedEntry;
