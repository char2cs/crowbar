use std::borrow::Borrow;

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(transparent)]
pub struct SpaceId(String);

impl SpaceId {
    #[must_use]
    pub fn new(id: impl Into<String>) -> Self {
        Self(id.into())
    }

    #[must_use]
    pub fn as_str(&self) -> &str {
        &self.0
    }

    #[must_use]
    pub fn is_unowned(&self) -> bool {
        self.0.is_empty()
    }
}

impl Borrow<str> for SpaceId {
    fn borrow(&self) -> &str {
        &self.0
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;

    #[test]
    fn a_space_is_looked_up_by_its_project_id_and_stored_as_a_string() {
        let shown: BTreeMap<SpaceId, u8> = [(SpaceId::new("p1"), 1)].into_iter().collect();
        assert_eq!(shown.get("p1"), Some(&1), "borrowed lookup");
        assert_eq!(
            serde_json::to_string(&SpaceId::new("p1")).unwrap(),
            "\"p1\"",
            "transparent"
        );
        assert!(SpaceId::default().is_unowned(), "empty is unowned");
        assert!(!SpaceId::new("p1").is_unowned(), "a project owns it");
    }
}
