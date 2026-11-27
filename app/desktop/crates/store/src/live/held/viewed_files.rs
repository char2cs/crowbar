use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use crate::live::{DiskFiles, Files, read_text};

const VIEWED_FILE: &str = "review-viewed.json";

pub(crate) type Marks = BTreeMap<String, String>;

#[derive(Debug, Default)]
pub(crate) struct ViewedFiles {
    file: PathBuf,
    by_workspace: BTreeMap<String, Marks>,
}

fn parsed(
    files: &dyn Files,
    file: &Path,
) -> Option<BTreeMap<String, Marks>> {
    let text = read_text(files, file)
        .inspect_err(|error| tracing::warn!(%error, "the viewed marks could not be read"))
        .ok()??;
    serde_json::from_str(&text)
        .inspect_err(|error| tracing::warn!(%error, "the viewed marks are not readable JSON"))
        .ok()
}

fn mark(
    marks: &mut Marks,
    (path, signature): &(String, Option<String>),
) {
    marks.remove(path);
    let Some(signature) = signature else {
        return;
    };
    marks.insert(path.clone(), signature.clone());
}

impl ViewedFiles {
    pub(crate) fn read(
        files: &dyn Files,
        file: PathBuf,
    ) -> Self {
        let by_workspace = parsed(files, &file).unwrap_or_default();
        Self { file, by_workspace }
    }

    pub(crate) fn in_dir(directory: &Path) -> Self {
        Self::read(&DiskFiles::default(), directory.join(VIEWED_FILE))
    }

    pub(crate) fn of(
        &self,
        workspace: &str,
    ) -> Marks {
        self.by_workspace
            .get(workspace)
            .cloned()
            .unwrap_or_default()
    }

    pub(crate) fn apply(
        &mut self,
        workspace: &str,
        changes: &[(String, Option<String>)],
    ) -> Marks {
        let marks = self.by_workspace.entry(workspace.to_owned()).or_default();
        for change in changes {
            mark(marks, change);
        }
        marks.clone()
    }

    pub(crate) fn save(
        &self,
        files: &dyn Files,
    ) {
        let written = serde_json::to_vec(&self.by_workspace)
            .map_err(std::io::Error::other)
            .and_then(|bytes| files.write(&self.file, &bytes));
        if let Err(error) = written {
            tracing::warn!(%error, "the viewed marks could not be saved");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeFiles;

    fn change(
        path: &str,
        signature: Option<&str>,
    ) -> (String, Option<String>) {
        (path.to_owned(), signature.map(str::to_owned))
    }

    #[test]
    fn marks_survive_a_reload_per_workspace_and_a_bad_file_reads_as_none() {
        let files = FakeFiles::default();
        let mut viewed = ViewedFiles::read(&files, "/state/review-viewed.json".into());
        viewed.apply(
            "w1",
            &[
                change("a.ts", Some("M||1|0")),
                change("b.ts", Some("A||2|0")),
            ],
        );
        let after = viewed.apply("w1", &[change("b.ts", None)]);
        viewed.apply("w2", &[change("z.ts", Some("D||0|3"))]);
        viewed.save(&files);
        let reloaded = ViewedFiles::read(&files, "/state/review-viewed.json".into());
        assert_eq!(after, reloaded.of("w1"));
        assert_eq!((reloaded.of("w1").len(), reloaded.of("w2").len()), (1, 1));
        files.put("/bad.json", b"{not json");
        assert_eq!(
            ViewedFiles::read(&files, "/bad.json".into()).of("w1"),
            Marks::new()
        );
    }
}
