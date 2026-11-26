//! Text scanners for paths and URLs in a logical line.

mod path_match;
mod paths;
mod urls;

pub(crate) use path_match::PathMatch;
pub(crate) use paths::find_paths;
pub(crate) use urls::address_runs;
pub(crate) use urls::find_urls;
