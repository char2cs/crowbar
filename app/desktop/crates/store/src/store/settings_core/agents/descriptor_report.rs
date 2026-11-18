//! The daemon's validation of one agent descriptor.

use daemonapi::Switch;
use serde::Deserialize;

use crate::store::settings_core::agents::descriptor_finding::DescriptorFinding;

pub(crate) const ERROR: &str = "error";

#[derive(Debug, Clone, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct DescriptorReport {
    pub(crate) id: String,
    pub(crate) source: String,
    pub(crate) findings: Vec<DescriptorFinding>,
    pub(crate) fell_back: Switch,
}

impl DescriptorReport {
    pub(crate) fn errors(&self) -> usize {
        self.findings
            .iter()
            .filter(|finding| finding.severity == ERROR)
            .count()
    }

    pub(crate) fn warnings(&self) -> usize {
        self.findings.len() - self.errors()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn findings_split_into_errors_and_warnings_and_a_missing_list_is_empty() {
        let report: DescriptorReport = serde_json::from_str(
            r#"{"id":"x","findings":[{"severity":"error"},{"severity":"warning"},{"severity":"warning"}]}"#,
        )
        .unwrap();
        assert_eq!((report.errors(), report.warnings()), (1, 2));
        let bare: DescriptorReport = serde_json::from_str(r#"{"id":"y","fellBack":true}"#).unwrap();
        assert_eq!(bare.findings.len(), 0);
        assert!(bare.fell_back.is_on());
    }
}
