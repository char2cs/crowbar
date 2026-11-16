//! The images of spaces fetched from the daemon, kept as files the sidebar can draw.

use std::collections::{BTreeMap, BTreeSet};
use std::path::{Path, PathBuf};

const MAX_NAME_PART: usize = 64;

fn is_file_safe(part: &str) -> bool {
    (1..=MAX_NAME_PART).contains(&part.len())
        && part
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b'-'))
}

#[derive(Debug, Default)]
pub(crate) struct IconCache {
    dir: PathBuf,
    held: BTreeMap<String, (String, PathBuf)>,
    asked: BTreeSet<(String, String)>,
}

impl IconCache {
    pub(crate) fn in_dir(dir: PathBuf) -> Self {
        Self {
            dir,
            ..Self::default()
        }
    }

    pub(crate) fn file_for(
        &self,
        project: &str,
        url: &str,
    ) -> Option<&Path> {
        self.held
            .get(project)
            .filter(|(held_url, _)| held_url == url)
            .map(|(_, path)| path.as_path())
    }

    fn path_of(
        &self,
        project: &str,
        url: &str,
    ) -> Option<PathBuf> {
        let version = url.rsplit_once("?v=").map_or("0", |(_, version)| version);
        (is_file_safe(project) && is_file_safe(version))
            .then(|| self.dir.join(format!("{project}-{version}")))
    }

    pub(crate) fn begin(
        &mut self,
        project: &str,
        url: &str,
    ) -> Option<PathBuf> {
        let path = self.path_of(project, url)?;
        let fresh = self.file_for(project, url).is_none()
            && self.asked.insert((project.to_owned(), url.to_owned()));
        fresh.then_some(path)
    }

    pub(crate) fn finish(
        &mut self,
        project: &str,
        url: &str,
        stored: Option<PathBuf>,
    ) {
        self.asked.remove(&(project.to_owned(), url.to_owned()));
        if let Some(path) = stored {
            self.held.insert(project.to_owned(), (url.to_owned(), path));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_icon_is_asked_for_once_until_it_lands_or_fails() {
        let mut cache = IconCache::in_dir(PathBuf::from("/c"));
        let url = "/v0/projects/p/icon?v=3";
        assert!(cache.begin("p", url).is_some());
        assert_eq!(cache.begin("p", url), None, "already in flight");
        cache.finish("p", url, None);
        assert!(cache.begin("p", url).is_some(), "a failure may be retried");
        let path = cache.path_of("p", url).unwrap();
        assert_eq!(path, PathBuf::from("/c/p-3"));
        cache.finish("p", url, Some(path.clone()));
        assert_eq!(cache.file_for("p", url), Some(path.as_path()));
        assert_eq!(cache.begin("p", url), None, "held");
        assert!(
            cache.begin("p", "/v0/projects/p/icon?v=4").is_some(),
            "a new version"
        );
        assert_eq!(cache.file_for("p", "/v0/projects/p/icon?v=4"), None);
    }

    #[test]
    fn a_daemon_text_that_could_leave_the_directory_is_never_turned_into_a_path() {
        let mut cache = IconCache::in_dir(PathBuf::from("/c"));
        let hostile = [
            ("p", "/v0/projects/p/icon?v=../../../.ssh/authorized_keys"),
            ("p", "/v0/projects/p/icon?v=a/b"),
            ("p", "/v0/projects/p/icon?v=..%2fx"),
            ("p", "/v0/projects/p/icon?v="),
            ("p", "/v0/projects/p/icon?v=a\\b"),
            ("../x", "/v0/projects/p/icon?v=1"),
            ("a/b", "/v0/projects/p/icon?v=1"),
            ("", "/v0/projects/p/icon?v=1"),
            ("/etc/passwd", "/v0/projects/p/icon?v=1"),
        ];
        for (project, url) in hostile {
            assert_eq!(cache.begin(project, url), None, "{project} {url}");
        }
        let long = "a".repeat(MAX_NAME_PART + 1);
        assert_eq!(cache.begin(&long, "/i?v=1"), None, "too long");
        assert_eq!(
            cache.begin("p", &format!("/i?v={long}")),
            None,
            "version too long"
        );
    }
}
