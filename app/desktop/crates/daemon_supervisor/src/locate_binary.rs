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
mod tests {
    use super::*;

    fn dir(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("cb-loc-{tag}-{}", std::process::id()));
        std::fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[test]
    fn an_explicit_path_wins_without_being_checked() {
        let found = locate_binary(Some(Path::new("/x/crowbar-api")), None, None).unwrap();
        assert_eq!(found, PathBuf::from("/x/crowbar-api"));
    }

    fn stock(
        places: &[PathBuf],
        flags: [bool; 2],
    ) {
        places
            .iter()
            .zip(flags)
            .filter(|(_, stocked)| *stocked)
            .for_each(|(place, _)| std::fs::write(place.join(BINARY_NAME), b"").unwrap());
    }

    #[test]
    fn the_binary_is_searched_beside_the_app_then_on_the_path() {
        let cases = [
            ("beside beats path", [true, true], Some(0)),
            ("path is searched", [false, true], Some(1)),
            ("a miss", [false, false], None),
        ];
        for (name, flags, winner) in cases {
            let places = [dir(&format!("{name}-app")), dir(&format!("{name}-path"))];
            stock(&places, flags);
            let exe = places[0].join("crowbar");
            let found = locate_binary(None, Some(&exe), Some(places[1].as_os_str()));
            let expected = winner.map(|at: usize| places[at].join(BINARY_NAME));
            assert_eq!(found.ok(), expected, "{name}");
        }
    }

    #[test]
    fn a_miss_names_every_place_looked() {
        let empty = dir("empty");
        let err =
            locate_binary(None, Some(&empty.join("crowbar")), Some(empty.as_os_str())).unwrap_err();
        assert!(err.to_string().contains("crowbar-api"), "{err}");
    }
}
