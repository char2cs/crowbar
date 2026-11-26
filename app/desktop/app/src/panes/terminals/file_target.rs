use std::path::Path;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum FileTarget {
    Workspace(String),
    Host(String),
    Outside(String),
}

impl FileTarget {
    pub(crate) fn of(
        absolute: &str,
        (root, remote_host): (Option<&Path>, Option<&str>),
    ) -> Self {
        let Some(root) = root else {
            return Self::Host(absolute.to_owned());
        };
        if let Some(relative) = inside(absolute, root) {
            return Self::Workspace(relative);
        }
        match remote_host {
            Some(_) => Self::Outside(absolute.to_owned()),
            None => Self::Host(absolute.to_owned()),
        }
    }
}

fn inside(
    absolute: &str,
    root: &Path,
) -> Option<String> {
    let relative = Path::new(absolute).strip_prefix(root).ok()?;
    let relative = relative.to_str()?;
    (!relative.is_empty()).then(|| relative.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_path_opens_in_its_workspace_on_this_computer_or_is_refused_on_another() {
        let root = Some(Path::new("/work/tree"));
        let cases = [
            (
                "/work/tree/src/a.ts",
                (root, None),
                FileTarget::Workspace("src/a.ts".into()),
            ),
            (
                "/work/tree/src/a.ts",
                (root, Some("far")),
                FileTarget::Workspace("src/a.ts".into()),
            ),
            (
                "/etc/hosts",
                (root, None),
                FileTarget::Host("/etc/hosts".into()),
            ),
            (
                "/etc/hosts",
                (root, Some("far")),
                FileTarget::Outside("/etc/hosts".into()),
            ),
            (
                "/work/treeish/a",
                (root, None),
                FileTarget::Host("/work/treeish/a".into()),
            ),
            (
                "/etc/hosts",
                (None, None),
                FileTarget::Host("/etc/hosts".into()),
            ),
        ];
        for (absolute, workspace, expected) in cases {
            assert_eq!(FileTarget::of(absolute, workspace), expected, "{absolute}");
        }
    }
}
