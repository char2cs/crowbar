const WORKSPACE_PREFIX: &str = "crowbar://editor/";
const HOST_PREFIX: &str = "crowbar://host/";
const UNRESERVED: &[u8] = b"-_.!~*'()";

#[must_use]
pub fn workspace_uri(
    workspace: &str,
    path: &str,
) -> String {
    format!(
        "{WORKSPACE_PREFIX}{}/{}",
        encode_component(workspace),
        encode_component(path)
    )
}

#[must_use]
pub fn host_uri(path: &str) -> String {
    format!("{HOST_PREFIX}{}", encode_component(path))
}

fn encode_component(text: &str) -> String {
    text.bytes().map(encoded).collect()
}

fn encoded(byte: u8) -> String {
    if byte.is_ascii_alphanumeric() || UNRESERVED.contains(&byte) {
        return char::from(byte).to_string();
    }
    format!("%{byte:02X}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn documents_are_named_as_develop_encodes_them() {
        assert_eq!(
            workspace_uri("ws 1", "src/a b.ts"),
            "crowbar://editor/ws%201/src%2Fa%20b.ts"
        );
        assert_eq!(host_uri("/tmp/é.txt"), "crowbar://host/%2Ftmp%2F%C3%A9.txt");
    }
}
