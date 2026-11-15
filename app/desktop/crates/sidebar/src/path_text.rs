//! Workspace-relative path arithmetic for the Files panel.

pub(crate) fn join(
    dir: &str,
    name: &str,
) -> String {
    if dir.is_empty() || dir == "." {
        return name.to_owned();
    }
    format!("{}/{name}", dir.trim_end_matches('/'))
}

pub(crate) fn parent(path: &str) -> &str {
    path.rsplit_once('/').map_or("", |(dir, _)| dir)
}

pub(crate) fn base(path: &str) -> &str {
    path.rsplit('/').next().unwrap_or(path)
}

pub(crate) fn absolute(
    root: Option<&str>,
    path: &str,
) -> String {
    let Some(root) = root else {
        return path.to_owned();
    };
    if path.is_empty() || path == "." {
        return root.to_owned();
    }
    join(root, path)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn paths_join_split_and_anchor_at_the_root() {
        type Case = (&'static str, String, &'static str);
        let cases: [Case; 10] = [
            ("root join", join("", "a.rs"), "a.rs"),
            ("nested join", join("src", "a.rs"), "src/a.rs"),
            ("trailing slash", join("src/", "a.rs"), "src/a.rs"),
            ("parent", parent("src/a.rs").to_owned(), "src"),
            ("top level parent", parent("a.rs").to_owned(), ""),
            ("base", base("src/a.rs").to_owned(), "a.rs"),
            ("file", absolute(Some("/w"), "src/a.rs"), "/w/src/a.rs"),
            ("root", absolute(Some("/w"), ""), "/w"),
            ("filesystem root", absolute(Some("/"), "a"), "/a"),
            ("unknown root", absolute(None, "src"), "src"),
        ];
        for (name, got, want) in cases {
            assert_eq!(got, want, "{name}");
        }
    }
}
