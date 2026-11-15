//! A remote branch as the daemon lists it.

use serde::Deserialize;

use crate::availability::Availability;
use crate::switch::Switch;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct BranchEntry {
    pub(crate) name: String,
    pub(crate) is_protected: Switch,
    pub(crate) has_workspace: Switch,
}

impl BranchEntry {
    pub(crate) fn availability(&self) -> Availability {
        if self.is_protected.is_on() {
            return Availability::Protected;
        }
        if self.has_workspace.is_on() {
            return Availability::Imported;
        }
        Availability::Importable
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, Availability); 4] = [
        (r#"{"name":"a"}"#, Availability::Importable),
        (
            r#"{"name":"a","hasWorkspace":true}"#,
            Availability::Imported,
        ),
        (
            r#"{"name":"a","isProtected":true}"#,
            Availability::Protected,
        ),
        (
            r#"{"name":"a","isProtected":true,"hasWorkspace":true}"#,
            Availability::Protected,
        ),
    ];

    #[test]
    fn a_branch_is_protected_before_it_is_imported_before_it_is_free() {
        for (json, expected) in CASES {
            let branch: BranchEntry = serde_json::from_str(json).unwrap();
            assert_eq!(branch.availability(), expected, "{json}");
        }
    }
}
