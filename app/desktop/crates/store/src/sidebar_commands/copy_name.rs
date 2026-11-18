//! The name a duplicate of a file takes in its folder.

use std::path::Path;

fn split(name: &str) -> (&str, String) {
    let path = Path::new(name);
    let stem = path
        .file_stem()
        .and_then(|stem| stem.to_str())
        .unwrap_or(name);
    let extension = path
        .extension()
        .and_then(|extension| extension.to_str())
        .map_or_else(String::new, |extension| format!(".{extension}"));
    (stem, extension)
}

fn candidate(
    (stem, extension): (&str, &str),
    number: usize,
) -> String {
    if number <= 1 {
        return format!("{stem} copy{extension}");
    }
    format!("{stem} copy {number}{extension}")
}

pub(crate) fn free(
    name: &str,
    taken: &[&str],
) -> String {
    let (stem, extension) = split(name);
    (1..=taken.len() + 1)
        .map(|number| candidate((stem, &extension), number))
        .find(|name| !taken.contains(&name.as_str()))
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (
        &'static str,
        &'static str,
        &'static [&'static str],
        &'static str,
    );

    const CASES: [Case; 5] = [
        ("a duplicate says copy", "a.rs", &["a.rs"], "a copy.rs"),
        (
            "a taken copy counts up",
            "a.rs",
            &["a.rs", "a copy.rs"],
            "a copy 2.rs",
        ),
        ("no extension", "Makefile", &["Makefile"], "Makefile copy"),
        ("a dotfile keeps its dot", ".env", &[".env"], ".env copy"),
        (
            "a gap is filled",
            "a.rs",
            &["a copy.rs", "a copy 3.rs"],
            "a copy 2.rs",
        ),
    ];

    #[test]
    fn a_duplicate_takes_the_first_free_copy_name() {
        for (why, name, taken, expected) in CASES {
            assert_eq!(free(name, taken), expected, "{why}");
        }
    }
}
