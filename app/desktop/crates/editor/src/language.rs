mod known;
mod tables;

use tables::{
    ANGULAR_SUFFIXES, ANGULAR_TEMPLATE, DOTENV, ENV_FILE, GIT_ATTRIBUTES, GIT_IGNORE,
    GIT_INFO_ATTRIBUTES, GIT_INFO_EXCLUDE, LANGUAGES, PLAIN_TEXT,
};

/// The language a document is written in, by develop's ids (`typescript`, `rust`, `text`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Language {
    id: &'static str,
}

impl Language {
    /// The language of the file at `path`, as develop's `getLanguageIdFromPath` resolves it;
    /// Plain Text when nothing names one.
    #[must_use]
    pub fn of_path(path: &str) -> Self {
        let id = by_whole_path(path)
            .or_else(|| by_file_name(path))
            .or_else(|| by_extension(path))
            .unwrap_or(PLAIN_TEXT);
        Self { id }
    }

    /// The language's id, as develop names it.
    #[must_use]
    pub fn id(self) -> &'static str {
        self.id
    }

    /// The name the toolbar's language chip shows ("TypeScript", "Plain Text").
    #[must_use]
    pub fn display_name(self) -> &'static str {
        LANGUAGES
            .iter()
            .find(|known| known.id == self.id)
            .map_or(self.id, |known| known.name)
    }
}

fn file_name(path: &str) -> String {
    path.rsplit('/').next().unwrap_or(path).to_lowercase()
}

fn by_whole_path(path: &str) -> Option<&'static str> {
    let lower = path.replace('\\', "/").to_lowercase();
    let name = file_name(path);
    if ANGULAR_SUFFIXES.iter().any(|suffix| name.ends_with(suffix)) {
        return Some(ANGULAR_TEMPLATE);
    }
    if lower.ends_with(GIT_INFO_EXCLUDE) {
        return Some(GIT_IGNORE);
    }
    if lower.ends_with(GIT_INFO_ATTRIBUTES) {
        return Some(GIT_ATTRIBUTES);
    }
    let env_file = name == ENV_FILE || name.starts_with(&format!("{ENV_FILE}."));
    env_file.then_some(DOTENV)
}

fn by_file_name(path: &str) -> Option<&'static str> {
    let name = file_name(path);
    LANGUAGES
        .iter()
        .find(|known| known.file_names.contains(&name.as_str()))
        .map(|known| known.id)
}

fn by_extension(path: &str) -> Option<&'static str> {
    let extension = path.rsplit('.').next().unwrap_or_default().to_lowercase();
    LANGUAGES
        .iter()
        .find(|known| known.extensions.contains(&extension.as_str()))
        .map(|known| known.id)
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str, &str); 9] = [
        ("src/pricing.ts", "typescript", "TypeScript"),
        ("web/App.TSX", "typescriptreact", "TSX"),
        ("README.md", "markdown", "Markdown"),
        ("notes", "text", "Plain Text"),
        (".env.local", "dotenv", "Dotenv"),
        ("app/x.component.html", "angular", "Angular Template"),
        ("repo/.git/info/exclude", "gitignore", "Git Ignore"),
        ("Dockerfile", "dockerfile", "Dockerfile"),
        ("go.mod", "go", "Go"),
    ];

    #[test]
    fn a_path_names_its_language_as_develop_does() {
        for (path, id, name) in CASES {
            let language = Language::of_path(path);
            assert_eq!(
                (language.id(), language.display_name()),
                (id, name),
                "{path}"
            );
        }
    }
}
