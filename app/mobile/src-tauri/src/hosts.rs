use crate::host_error::HostError;
use crate::host_route::is_remote_host;

pub(crate) mod endpoint;
mod host_registry;

pub(crate) use host_registry::HostRegistry;

pub(crate) fn validate_host_id(host_id: &str) -> Result<(), HostError> {
    let well_formed = host_id
        .bytes()
        .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-');
    if !well_formed {
        return Err(HostError::HostIdCharacters(host_id.to_owned()));
    }
    if !is_remote_host(host_id) {
        return Err(HostError::HostIdIsLocal(host_id.to_owned()));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_ids_the_router_would_send_to_that_host_are_accepted() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("", Some("names the local daemon")),
            ("localhost", Some("names the local daemon")),
            ("Host-1", Some("may only hold")),
            ("host@1", Some("may only hold")),
            ("host:1", Some("may only hold")),
            ("host/1", Some("may only hold")),
            ("host.1", Some("may only hold")),
            ("host 1", Some("may only hold")),
            ("host-1", None),
            ("0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43", None),
        ];
        for (host_id, refusal) in CASES {
            let reason = validate_host_id(host_id).err().map(|e| e.to_string());
            let matches = match (reason.as_deref(), refusal) {
                (Some(text), Some(expected)) => text.contains(expected),
                (None, None) => true,
                _ => false,
            };
            assert!(matches, "{host_id:?}: {reason:?}");
        }
    }
}
