use std::path::{Component, Path, PathBuf};

pub(crate) fn reveal_target(
    root: &Path,
    relative: &str,
) -> Option<PathBuf> {
    let inside = Path::new(relative)
        .components()
        .all(|part| matches!(part, Component::Normal(_) | Component::CurDir));
    inside.then(|| root.join(relative))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_path_stays_inside_the_workspace_root() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("src/lib.rs", Some("/ws/src/lib.rs")),
            ("src", Some("/ws/src")),
            ("", Some("/ws/")),
            ("../etc", None),
            ("/etc", None),
            ("a/../../b", None),
        ];
        for (relative, expected) in CASES {
            let got = reveal_target(Path::new("/ws"), relative);
            assert_eq!(got, expected.map(PathBuf::from), "{relative:?}");
        }
    }
}
