//! The images of spaces fetched from the daemon, kept as files the sidebar can draw.

use std::collections::{BTreeMap, BTreeSet};
use std::path::{Path, PathBuf};

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
    ) -> PathBuf {
        let version = url.rsplit_once("?v=").map_or("0", |(_, version)| version);
        self.dir.join(format!("{project}-{version}"))
    }

    pub(crate) fn begin(
        &mut self,
        project: &str,
        url: &str,
    ) -> Option<PathBuf> {
        let fresh = self.file_for(project, url).is_none()
            && self.asked.insert((project.to_owned(), url.to_owned()));
        fresh.then(|| self.path_of(project, url))
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
        let path = cache.path_of("p", url);
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
}
