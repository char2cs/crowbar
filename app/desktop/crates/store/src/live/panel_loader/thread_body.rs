use serde::Deserialize;

use crate::live::held::OpenThreads;

#[derive(Debug, Default, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub(crate) struct ThreadBody {
    pub(crate) resolved: bool,
    pub(crate) file_path: String,
}

pub(crate) fn open_in(threads: &[ThreadBody]) -> OpenThreads {
    let mut open = OpenThreads::default();
    for thread in threads.iter().filter(|thread| !thread.resolved) {
        open.count(&thread.file_path);
    }
    open
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_unresolved_threads_are_open() {
        let threads: Vec<ThreadBody> = serde_json::from_str(
            r#"[{"resolved":true,"filePath":"a"},{"resolved":false,"filePath":"a"},{}]"#,
        )
        .unwrap();
        let open = open_in(&threads);
        assert_eq!(open.total(), 2);
        assert_eq!(open.by_file().get("a"), Some(&1));
    }
}
