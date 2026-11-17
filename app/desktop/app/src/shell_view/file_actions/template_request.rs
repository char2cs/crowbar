use shell_data::Overwrite;

pub(super) struct TemplateRequest<'a> {
    pub(super) source: &'a str,
    pub(super) target: &'a str,
    pub(super) overwrite: Overwrite,
}

impl<'a> TemplateRequest<'a> {
    pub(super) fn asking(
        source: &'a str,
        target: &'a str,
    ) -> Self {
        Self {
            source,
            target,
            overwrite: Overwrite::Ask,
        }
    }

    pub(super) fn replacing(
        source: &'a str,
        target: &'a str,
    ) -> Self {
        Self {
            source,
            target,
            overwrite: Overwrite::Replace,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_request_either_asks_before_replacing_or_replaces() {
        let asking = TemplateRequest::asking(".env", ".env.example");
        let replacing = TemplateRequest::replacing(".env", ".env.example");
        assert!(matches!(asking.overwrite, Overwrite::Ask), "asking");
        assert!(
            matches!(replacing.overwrite, Overwrite::Replace),
            "replacing"
        );
        assert_eq!((asking.source, asking.target), (".env", ".env.example"));
    }
}
