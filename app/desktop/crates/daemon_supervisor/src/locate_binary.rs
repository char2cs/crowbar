use std::ffi::OsStr;
use std::path::{Path, PathBuf};

use crate::error::Error;

const BINARY_NAME: &str = "crowbar-api";

fn is_executable_file(path: &Path) -> bool {
    path.metadata().is_ok_and(|meta| meta.is_file())
}

/// Finds the daemon binary: `explicit` when given, else `crowbar-api` beside `current_exe`, else
/// on `path_var`. A bundled app keeps the daemon next to its own executable.
///
/// # Errors
/// [`Error::BinaryNotFound`] naming every place that was looked in.
pub fn locate_binary(
    explicit: Option<&Path>,
    current_exe: Option<&Path>,
    path_var: Option<&OsStr>,
) -> Result<PathBuf, Error> {
    if let Some(path) = explicit {
        return Ok(path.to_path_buf());
    }
    let beside = current_exe
        .and_then(Path::parent)
        .map(|dir| dir.join(BINARY_NAME));
    let on_path = path_var
        .into_iter()
        .flat_map(std::env::split_paths)
        .map(|dir| dir.join(BINARY_NAME));
    let candidates: Vec<PathBuf> = beside.into_iter().chain(on_path).collect();
    candidates
        .iter()
        .find(|candidate| is_executable_file(candidate))
        .cloned()
        .ok_or_else(|| Error::BinaryNotFound {
            looked: candidates
                .iter()
                .map(|c| c.display().to_string())
                .collect::<Vec<_>>()
                .join(", "),
        })
}

#[cfg(test)]
#[path = "locate_binary_tests.rs"]
mod tests;
