/// Why a language server request failed: the daemon refused it or could not be reached.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LspError(pub String);

impl std::fmt::Display for LspError {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl std::error::Error for LspError {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_failed_request_reaches_a_provider_s_result_with_its_reason() {
        let failed: gpui::Result<()> = Err(LspError("refused".to_owned()).into());
        let reason = failed.map_err(|error| error.to_string());
        assert_eq!(reason, Err("refused".to_owned()));
    }
}
