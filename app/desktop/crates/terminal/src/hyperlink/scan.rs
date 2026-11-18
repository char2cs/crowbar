//! Text scanners for paths and URLs in a logical line.

mod address_runs;
mod path_match;
mod paths;
mod urls;

pub(crate) use address_runs::address_runs;
pub(crate) use path_match::PathMatch;
pub(crate) use paths::find_paths;
pub(crate) use urls::find_urls;
