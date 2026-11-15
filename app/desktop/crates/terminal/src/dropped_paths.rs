//! Files dropped on the terminal, as text for the shell.

use std::path::PathBuf;

const SPECIAL: [char; 5] = ['"', '\\', '$', '`', '\''];

/// The shell text for dropped `paths`: absolute paths only, quoted when they need it, each followed
/// by a space. Empty when no path qualifies.
pub fn format_dropped_paths(paths: &[PathBuf]) -> String {
    paths
        .iter()
        .filter(|path| path.is_absolute())
        .filter_map(|path| path.to_str())
        .map(|path| quote(path) + " ")
        .collect()
}

fn quote(path: &str) -> String {
    let needs_quotes = path.contains(char::is_whitespace) || path.contains(SPECIAL);
    if !needs_quotes {
        return path.to_owned();
    }
    let escaped: String = path
        .chars()
        .flat_map(|character| {
            let escape = matches!(character, '"' | '\\' | '$' | '`');
            escape
                .then_some('\\')
                .into_iter()
                .chain(std::iter::once(character))
        })
        .collect();
    format!("\"{escaped}\"")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn absolute_paths_are_quoted_when_needed() {
        let paths = [
            PathBuf::from("/a/b.txt"),
            PathBuf::from("/a b/$c"),
            PathBuf::from("relative/x"),
        ];
        assert_eq!(format_dropped_paths(&paths), "/a/b.txt \"/a b/\\$c\" ");
        assert_eq!(format_dropped_paths(&[]), "");
    }
}
