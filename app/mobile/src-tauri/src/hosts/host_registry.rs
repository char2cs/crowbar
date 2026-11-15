use std::collections::HashMap;
use std::sync::Mutex;

use super::endpoint::Endpoint;
use super::validate_host_id;

#[derive(Default)]
pub(crate) struct HostRegistry(Mutex<HashMap<String, Endpoint>>);

impl HostRegistry {
    pub(crate) fn get(
        &self,
        host_id: &str,
    ) -> Option<Endpoint> {
        self.0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .get(host_id)
            .cloned()
    }

    pub(crate) fn insert(
        &self,
        host_id: String,
        endpoint: Endpoint,
    ) -> Result<(), String> {
        validate_host_id(&host_id)?;
        self.0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(host_id, endpoint);
        Ok(())
    }

    pub(crate) fn remove(
        &self,
        host_id: &str,
    ) {
        self.0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(host_id);
    }
}

#[cfg(test)]
mod tests {
    use super::super::endpoint::parse_endpoint;
    use super::*;

    const ENDPOINT: &str = "1.2.3.4:3737";

    fn at(endpoint: &str) -> Endpoint {
        parse_endpoint(endpoint).unwrap()
    }

    #[test]
    fn insert_get_and_remove_follow_the_registry_contents() {
        let reg = HostRegistry::default();
        assert!(reg.get("host-1").is_none(), "unknown host");
        reg.insert("host-1".into(), at(ENDPOINT)).unwrap();
        assert_eq!(reg.get("host-1"), Some(at(ENDPOINT)));
        reg.remove("host-1");
        assert!(reg.get("host-1").is_none(), "removed host is unreachable");
    }

    #[test]
    fn a_refused_id_is_never_registered() {
        const CASES: &[(&str, bool)] = &[
            ("", false),
            ("localhost", false),
            ("Host-1", false),
            ("host 1", false),
            ("host-1", true),
            ("0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43", true),
        ];
        let reg = HostRegistry::default();
        for (host_id, accepted) in CASES {
            let outcome = reg.insert((*host_id).into(), at(ENDPOINT));
            assert_eq!(outcome.is_ok(), *accepted, "{host_id:?}");
            assert_eq!(reg.get(host_id).is_some(), *accepted, "{host_id:?}");
        }
    }
}
