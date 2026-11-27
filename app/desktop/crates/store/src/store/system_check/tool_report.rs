use serde::Deserialize;

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct ToolReport {
    #[serde(default)]
    pub installed: bool,
    #[serde(default)]
    pub authed: bool,
    #[serde(default)]
    pub version: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_daemons_two_answer_shapes_both_read() {
        let git: ToolReport =
            serde_json::from_str(r#"{"installed":true,"version":"git version 2.45"}"#).unwrap();
        assert_eq!(
            git,
            ToolReport {
                installed: true,
                authed: false,
                version: Some("git version 2.45".into())
            }
        );
        let gh: ToolReport = serde_json::from_str(r#"{"installed":true,"authed":true}"#).unwrap();
        assert!(gh.installed && gh.authed && gh.version.is_none());
    }
}
