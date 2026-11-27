use serde::Deserialize;

const STRUCTURAL: [&str; 3] = ["created", "deleted", "renamed"];
const TEXTUAL: [&str; 2] = ["modified", "created"];

#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct FileChange {
    #[serde(rename = "type", default)]
    pub(crate) change: String,
    #[serde(default)]
    pub(crate) path: String,
    #[serde(default)]
    pub(crate) new_path: Option<String>,
}

impl FileChange {
    pub(crate) fn changes_text(&self) -> bool {
        !self.path.is_empty() && TEXTUAL.contains(&self.change.as_str())
    }

    pub(crate) fn folders(&self) -> Vec<String> {
        if self.path.is_empty() || !STRUCTURAL.contains(&self.change.as_str()) {
            return Vec::new();
        }
        let moved = self.new_path.iter().map(|path| parent_of(path));
        let mut folders: Vec<String> = std::iter::once(parent_of(&self.path))
            .chain(moved)
            .collect();
        folders.dedup();
        folders
    }
}

fn parent_of(path: &str) -> String {
    path.rsplit_once('/')
        .map_or_else(String::new, |(parent, _)| parent.to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn change(json: &str) -> FileChange {
        serde_json::from_str(json).unwrap()
    }

    #[test]
    fn a_structural_change_names_the_folders_it_alters_and_an_edit_none() {
        let deleted = change(r#"{"type":"deleted","path":"src/a.ts"}"#);
        let created = change(r#"{"type":"created","path":"a.ts"}"#);
        let renamed = change(r#"{"type":"renamed","path":"src/a.ts","newPath":"lib/b.ts"}"#);
        let within = change(r#"{"type":"renamed","path":"src/a.ts","newPath":"src/b.ts"}"#);
        let edited = change(r#"{"type":"modified","path":"src/a.ts"}"#);
        assert_eq!(deleted.folders(), ["src"]);
        assert_eq!(created.folders(), [""]);
        assert_eq!(renamed.folders(), ["src", "lib"]);
        assert_eq!(within.folders(), ["src"]);
        assert_eq!(edited.folders(), Vec::<String>::new());
    }
}
