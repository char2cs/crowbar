use std::path::Path;

pub(crate) fn terminal_root(
    directory: Option<&str>,
    workspace: Option<&Path>,
) -> Option<String> {
    let opened_in = directory
        .filter(|directory| !directory.is_empty())
        .and_then(|directory| folder_of(directory, workspace));
    opened_in.or_else(|| workspace.and_then(Path::to_str).map(str::to_owned))
}

fn folder_of(
    directory: &str,
    workspace: Option<&Path>,
) -> Option<String> {
    if directory.starts_with('/') {
        return Some(directory.to_owned());
    }
    workspace?.join(directory).to_str().map(str::to_owned)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn links_resolve_in_the_opened_folder_else_the_workspace_root() {
        let root = Some(Path::new("/work/tree"));
        let cases = [
            (Some("/abs/dir"), root, Some("/abs/dir")),
            (Some("src"), root, Some("/work/tree/src")),
            (Some(""), root, Some("/work/tree")),
            (None, root, Some("/work/tree")),
            (Some("src"), None, None),
            (None, None, None),
        ];
        for (directory, workspace, expected) in cases {
            let got = terminal_root(directory, workspace);
            assert_eq!(got.as_deref(), expected, "{directory:?}");
        }
    }
}
