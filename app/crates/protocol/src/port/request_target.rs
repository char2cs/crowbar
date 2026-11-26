use crate::error::{Error, Result};
use crate::string_id::string_id;

const MAX_TARGET_BYTES: usize = 8 * 1024;

fn invalid(reason: &'static str) -> Error {
    Error::invalid_request(reason)
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
    if has_dot_segment(value) {
        return Err(invalid(
            "the request target holds a `.` or `..` path segment",
        ));
    }
    Ok(())
}

fn has_dot_segment(value: &str) -> bool {
    let path = value.split_once('?').map_or(value, |(path, _)| path);
    path.split('/').any(|segment| {
        let decoded = segment.to_ascii_lowercase().replace("%2e", ".");
        decoded == "." || decoded == ".." || segment.to_ascii_lowercase().contains("%2f")
    })
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
        for ok in [
            "/",
            "/v0/health",
            "/v0/threads?limit=5&q=a%20b",
            "/v0/x?v=../y",
            "/v0/a..b",
        ] {
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
            "/v0/../etc",
            "/v0/./x",
            "/v0/%2e%2E/x",
            "/v0/a%2Fb",
            "/..",
        ] {
            assert!(RequestTarget::new(bad).is_err(), "{bad:?}");
        }
        assert!(
            RequestTarget::new(format!("/{}", "a".repeat(MAX_TARGET_BYTES))).is_err(),
            "too long"
        );
    }
}
