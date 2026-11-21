//! The parts a chats row is built from: clusters, verbs, glyphs, labels, the shell, focus, input, reports and bounds.

mod chat_glyph;
mod cluster_spec;
mod row_bounds;
mod row_cluster;
mod row_focus;
mod row_glyph;
mod row_hover;
mod row_input;
mod row_label;
mod row_report;
mod row_shell;
mod row_verb;
mod trailing;

pub use chat_glyph::{ChatGlyph, GlyphTone};
pub(crate) use cluster_spec::ClusterSpec;
pub(crate) use row_bounds::RowBounds;
pub(crate) use row_cluster::cluster;
pub(crate) use row_focus::{ringed, row_focus};
pub(crate) use row_glyph::RowGlyph;
pub(crate) use row_hover::{hovered_by, row_hover};
pub(crate) use row_input::RowInput;
pub(crate) use row_label::{input_view, label};
pub(crate) use row_report::report;
pub(crate) use row_shell::row_shell;
pub(crate) use trailing::Trailing;
