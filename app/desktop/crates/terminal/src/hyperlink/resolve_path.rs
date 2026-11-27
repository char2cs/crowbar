pub fn resolve_file_path(
    path: &str,
    root: Option<&str>,
) -> Option<String> {
    if path.starts_with('~') {
        return None;
    }
    if path.starts_with('/') {
        return Some(path.to_owned());
    }
    let root = root.filter(|root| root.starts_with('/'))?;
    let mut parts: Vec<&str> = root.split('/').collect();
    for segment in path.split('/') {
        fold_segment(&mut parts, segment);
    }
    Some(parts.join("/"))
}

fn fold_segment<'a>(
    parts: &mut Vec<&'a str>,
    segment: &'a str,
) {
    match segment {
        ".." if parts.len() > 1 => {
            parts.pop();
        },
        "" | "." | ".." => {},
        other => parts.push(other),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn paths_resolve_against_the_root() {
        let root = Some("/work/app");
        let cases = [
            ("/etc/hosts", root, Some("/etc/hosts")),
            ("src/main.rs", root, Some("/work/app/src/main.rs")),
            ("./a/../b.rs", root, Some("/work/app/b.rs")),
            ("../x", root, Some("/work/x")),
            ("~/x", root, None),
            ("src/x", None, None),
            ("src/x", Some("relative"), None),
        ];
        for (path, root, expected) in cases {
            assert_eq!(resolve_file_path(path, root).as_deref(), expected, "{path}");
        }
    }
}
