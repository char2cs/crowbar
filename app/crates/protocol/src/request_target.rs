use crate::error::{Error, Result};
use crate::string_id::string_id;

/// Longest request target accepted, in bytes.
pub const MAX_TARGET_BYTES: usize = 8 * 1024;

fn invalid(reason: &'static str) -> Error {
    Error::InvalidRequest { reason }
}

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_TARGET_BYTES {
        return Err(invalid("the request target is too long"));
    }
    if !value.starts_with('/') || value.starts_with("//") {
        return Err(invalid("the request target must start with a single `/`"));
    }
    if !value.bytes().all(|byte| byte.is_ascii_graphic()) {
        return Err(invalid(
            "the request target holds spaces, controls or non-ASCII bytes",
        ));
    }
    if value.contains('\\') || value.contains('#') {
        return Err(invalid(
            "the request target holds a backslash or a fragment",
        ));
    }
    Ok(())
}

string_id!(
    /// The path and query of a daemon request, for example `/v0/threads?limit=5`. It starts with
    /// exactly one `/`, so it can never change the host a request is addressed to, and holds no
    /// spaces, control characters, backslash or fragment.
    RequestTarget,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_absolute_paths_with_queries() {
        for ok in ["/", "/v0/health", "/v0/threads?limit=5&q=a%20b"] {
            assert!(RequestTarget::new(ok).is_ok(), "{ok}");
        }
    }

    #[test]
    fn refuses_anything_that_could_name_another_host_or_split_the_request() {
        for bad in [
            "",
            "v0/x",
            "@host-2/v0/x",
            "-x/v0/x",
            ":80/v0/x",
            ".evil/v0/x",
            "\\\\host\\x",
            "//host-2/v0/x",
            "/v0/a b",
            "/v0/a\r\nHost: x",
            "/v0/\u{e9}",
            "/v0/x#frag",
            "/v0\\x",
        ] {
            assert!(RequestTarget::new(bad).is_err(), "{bad:?}");
        }
        assert!(
            RequestTarget::new(format!("/{}", "a".repeat(MAX_TARGET_BYTES))).is_err(),
            "too long"
        );
    }
}
