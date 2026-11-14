use protocol::HostId;

use crate::error::Error;
use crate::host_target::{HostTarget, check_registrable};

const SCHEME_PREFIX: &str = "crowbar://";
const LOCAL_AUTHORITY: &str = "localhost";

#[derive(Debug, Clone, PartialEq, Eq)]
/// A path split into the host it addresses and the path on that host.
pub struct HostPath {
    /// The host the path addresses.
    pub target: HostTarget,
    /// The path on that host, starting with `/` for a remote host.
    pub path: String,
}

fn bad(path: &str, why: &'static str) -> Error {
    Error::BadHostPath {
        path: path.to_owned(),
        why,
    }
}

fn is_qualified(input: &str) -> bool {
    let trimmed = input.trim_start();
    trimmed
        .get(..SCHEME_PREFIX.len())
        .is_some_and(|head| head.eq_ignore_ascii_case(SCHEME_PREFIX))
}

fn check_remote_path(path: &str) -> Result<(), Error> {
    if !path.starts_with('/') || path.starts_with("//") {
        return Err(bad(path, "must start with a single \"/\""));
    }
    Ok(())
}

/// Qualifies `path` for `target`: a remote path becomes `crowbar://<host><path>`, a local path
/// stays as it is. Mirrors `qualifyHostPath` in `app/panes/src/lib/host-path.ts`.
///
/// # Errors
/// [`Error::BadHostPath`] for a remote path that is not absolute or a local path already qualified.
pub fn qualify(target: &HostTarget, path: &str) -> Result<String, Error> {
    match target {
        HostTarget::Local if is_qualified(path) => {
            Err(bad(path, "a local path is never qualified"))
        },
        HostTarget::Local => Ok(path.to_owned()),
        HostTarget::Remote(id) => {
            check_remote_path(path)?;
            Ok(format!("{SCHEME_PREFIX}{id}{path}"))
        },
    }
}

/// Splits a possibly qualified path into its host and path. Mirrors `splitHostPath` in
/// `app/panes/src/lib/host-path.ts`.
///
/// # Errors
/// [`Error::BadHostPath`] for a malformed scheme, an empty or invalid host or a bad path.
pub fn split_host_path(input: &str) -> Result<HostPath, Error> {
    if is_qualified(input) && !input.starts_with(SCHEME_PREFIX) {
        return Err(bad(input, "the scheme must be exactly \"crowbar://\""));
    }
    let Some(rest) = input.strip_prefix(SCHEME_PREFIX) else {
        return Ok(HostPath {
            target: HostTarget::Local,
            path: input.to_owned(),
        });
    };
    let (authority, path) = rest.split_at(rest.find('/').unwrap_or(rest.len()));
    if authority == LOCAL_AUTHORITY {
        return Ok(HostPath {
            target: HostTarget::Local,
            path: path.to_owned(),
        });
    }
    let id = HostId::new(authority).map_err(|_| bad(input, "the authority is not a host id"))?;
    check_registrable(&id)?;
    check_remote_path(path)?;
    Ok(HostPath {
        target: HostTarget::Remote(id),
        path: path.to_owned(),
    })
}

#[cfg(test)]
#[path = "host_path_tests.rs"]
mod tests;
