use crate::route::query::encode;

pub(crate) fn content(
    workspace: &str,
    path: &str,
) -> String {
    format!(
        "/v0/documents/content?workspace={}&path={}",
        encode(workspace),
        encode(path)
    )
}

pub(crate) fn host_content(path: &str) -> String {
    format!("/v0/documents/content?path={}", encode(path))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_workspace_and_path_are_encoded() {
        assert_eq!(
            content("w 1", "a/b.txt"),
            "/v0/documents/content?workspace=w%201&path=a/b.txt"
        );
        assert_eq!(
            host_content("/etc/my hosts"),
            "/v0/documents/content?path=/etc/my%20hosts"
        );
    }
}
