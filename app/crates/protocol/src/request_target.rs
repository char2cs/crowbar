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
#[path = "request_target_tests.rs"]
mod tests;
