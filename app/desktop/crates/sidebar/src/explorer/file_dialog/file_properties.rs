use crate::shell::PathText;

const NO_EXTENSION: &str = "No extension";

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FileProperties {
    pub file_name: String,
    pub path: String,
    pub kind: String,
}

impl FileProperties {
    pub fn of(path: &str) -> Self {
        let file_name = PathText::new(path).base();
        let kind = file_name
            .rsplit_once('.')
            .map(|(_, extension)| extension)
            .filter(|extension| !extension.is_empty())
            .unwrap_or(NO_EXTENSION);
        Self {
            file_name: file_name.to_owned(),
            path: path.to_owned(),
            kind: kind.to_owned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_type_is_the_extension_or_none() {
        let cases = [
            ("src/main.rs", "main.rs", "rs"),
            (".env", ".env", "env"),
            ("Makefile", "Makefile", "No extension"),
            ("a.", "a.", "No extension"),
        ];
        for (path, name, kind) in cases {
            let properties = FileProperties::of(path);
            assert_eq!(properties.file_name, name, "{path}");
            assert_eq!(properties.kind, kind, "{path}");
            assert_eq!(properties.path, path, "{path}");
        }
    }
}
