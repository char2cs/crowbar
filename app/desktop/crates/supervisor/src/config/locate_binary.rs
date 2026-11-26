use std::ffi::OsStr;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};

use crate::error::Error;

const BINARY_NAME: &str = "crowbar-api";
const EXECUTE_BITS: u32 = 0o111;

fn is_executable_file(path: &Path) -> bool {
    path.metadata()
        .is_ok_and(|meta| meta.is_file() && meta.permissions().mode() & EXECUTE_BITS != 0)
}

/// Finds the daemon binary: `explicit` when given, else `crowbar-api` beside `current_exe`, else
/// in the absolute directories of `path_var`, in that order. Relative or empty `path_var`
/// entries are skipped so the working directory is never searched. A bundled app keeps the
/// daemon next to its own executable, and callers that must trust the binary pass no `path_var`.
///
/// # Errors
/// [`Error::BinaryNotFound`] naming every place that was looked in.
pub fn locate_binary(
    explicit: Option<&Path>,
    current_exe: Option<&Path>,
    path_var: Option<&OsStr>,
) -> Result<PathBuf, Error> {
    locate_where(explicit, current_exe, path_var, is_executable_file)
}

fn locate_where(
    explicit: Option<&Path>,
    current_exe: Option<&Path>,
    path_var: Option<&OsStr>,
    is_executable: impl Fn(&Path) -> bool,
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
        .filter(|dir| dir.is_absolute())
        .map(|dir| dir.join(BINARY_NAME));
    let candidates: Vec<PathBuf> = beside.into_iter().chain(on_path).collect();
    candidates
        .iter()
        .find(|candidate| is_executable(candidate))
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
mod tests {
    use super::*;

    const APP: &str = "/app";
    const ON_PATH: &str = "/usr/bin";

    fn at(dir: &str) -> PathBuf {
        PathBuf::from(dir).join(BINARY_NAME)
    }

    fn search(
        explicit: Option<&str>,
        executable: &[PathBuf],
    ) -> Result<PathBuf, Error> {
        let exe = Path::new(APP).join("crowbar");
        let path_var = Some(OsStr::new(ON_PATH));
        locate_where(explicit.map(Path::new), Some(&exe), path_var, |candidate| {
            executable.iter().any(|path| path == candidate)
        })
    }

    #[test]
    fn the_binary_is_explicit_or_searched_beside_the_app_then_on_the_path() {
        let explicit = PathBuf::from("/x/crowbar-api");
        let cases = [
            ("explicit wins unchecked", Some("/x/crowbar-api"), vec![], Some(explicit)),
            ("beside beats path", None, vec![at(APP), at(ON_PATH)], Some(at(APP))),
            ("path is searched", None, vec![at(ON_PATH)], Some(at(ON_PATH))),
            ("a miss", None, vec![], None),
        ];
        for (name, explicit, executable, winner) in cases {
            assert_eq!(search(explicit, &executable).ok(), winner, "{name}");
        }
    }

    #[test]
    fn a_miss_names_every_place_looked() {
        let message = search(None, &[]).unwrap_err().to_string();
        for place in [at(APP), at(ON_PATH)] {
            assert!(message.contains(&place.display().to_string()), "{message}");
        }
    }

    #[test]
    fn a_relative_path_entry_is_never_searched() {
        let relative = std::env::join_paths(["", ".", "relative/bin"]).unwrap();
        let found = locate_where(None, None, Some(&relative), |_| true);
        assert!(found.is_err(), "relative entries");
    }

    #[test]
    #[ignore = "real-infra: reads the execute bit of a real file"]
    fn a_file_without_the_execute_bit_is_not_executable() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join(BINARY_NAME);
        std::fs::write(&file, b"").unwrap();
        std::fs::set_permissions(&file, std::fs::Permissions::from_mode(0o644)).unwrap();
        assert!(!is_executable_file(&file));
        std::fs::set_permissions(&file, std::fs::Permissions::from_mode(0o755)).unwrap();
        assert!(is_executable_file(&file));
    }
}
