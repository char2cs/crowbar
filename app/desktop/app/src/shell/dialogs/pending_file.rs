#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum PendingFile {
    OpenAll(Vec<String>),
    Overwrite { path: String, target: String },
}

impl PendingFile {
    pub(crate) fn title(&self) -> &'static str {
        match self {
            Self::OpenAll(_) => "Open All Files",
            Self::Overwrite { .. } => "Overwrite Env File",
        }
    }

    pub(crate) fn message(&self) -> String {
        match self {
            Self::OpenAll(files) => {
                format!("{} files will be opened in tabs. Continue?", files.len())
            },
            Self::Overwrite { target, .. } => format!("{target} already exists. Overwrite it?"),
        }
    }

    pub(crate) fn confirm_label(&self) -> &'static str {
        match self {
            Self::OpenAll(_) => "Open",
            Self::Overwrite { .. } => "Overwrite",
        }
    }

    pub(crate) fn destroys(&self) -> bool {
        matches!(self, Self::Overwrite { .. })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn overwrite() -> PendingFile {
        PendingFile::Overwrite {
            path: "app/.env".into(),
            target: ".env.example".into(),
        }
    }

    #[test]
    fn each_question_has_its_words_and_only_overwriting_is_destructive() {
        let many = PendingFile::OpenAll(vec!["a".into(); 120]);
        let cases = [
            (
                many,
                "Open All Files",
                "120 files will be opened in tabs. Continue?",
                "Open",
                false,
            ),
            (
                overwrite(),
                "Overwrite Env File",
                ".env.example already exists. Overwrite it?",
                "Overwrite",
                true,
            ),
        ];
        for (pending, title, message, label, destroys) in cases {
            assert_eq!(pending.title(), title);
            assert_eq!(pending.message(), message);
            assert_eq!(pending.confirm_label(), label);
            assert_eq!(pending.destroys(), destroys);
        }
    }
}
