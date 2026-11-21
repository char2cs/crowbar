mod document;
mod query;

use std::fmt;

use crate::route::query::encode;

/// The path of a daemon route, built from the ids it names.
///
/// A call takes it as its path ([`Call::get`](crate::Call::get) and the other verbs); it
/// displays as the path, so a caller can extend it with `format!`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Route {
    path: String,
}

impl Route {
    fn at(path: String) -> Self {
        Self { path }
    }

    /// The route of the project `project`.
    pub fn project(project: &str) -> Self {
        Self::at(format!("/v0/projects/{project}"))
    }

    /// The route of the repositories of the project `project_id`.
    pub fn repos(project_id: &str) -> Self {
        Self::at(format!("{}/repos", Self::project(project_id)))
    }

    /// The route prefix of the repository `repo` in the project `project_id`.
    ///
    /// An empty `repo` names the project home instead.
    pub fn scope(
        project_id: &str,
        repo: &str,
    ) -> Self {
        let base = Self::project(project_id);
        if repo.is_empty() {
            return Self::at(format!("{base}/home"));
        }
        Self::at(format!("{base}/repos/{repo}"))
    }

    /// The route of the chats of a repository or project home, as [`Route::scope`] names it.
    pub fn chats(
        project_id: &str,
        repo: &str,
    ) -> Self {
        Self::at(format!("{}/chats", Self::scope(project_id, repo)))
    }

    /// The route of the chat folders of a repository or project home, as [`Route::scope`] names
    /// it.
    pub fn folders(
        project_id: &str,
        repo: &str,
    ) -> Self {
        Self::at(format!("{}/folders", Self::chats(project_id, repo)))
    }

    /// The route of the content of the document at `path` in `workspace`, both percent-encoded.
    pub fn document_content(
        workspace: &str,
        path: &str,
    ) -> Self {
        Self::at(document::content(workspace, path))
    }

    /// The route of the content of the host's file at the absolute `path`, percent-encoded.
    pub fn host_document_content(path: &str) -> Self {
        Self::at(document::host_content(path))
    }

    /// The route listing the folder `path` of the daemon's machine, percent-encoded.
    pub fn browse(path: &str) -> Self {
        Self::at(format!("/v0/system/browse?path={}", encode(path)))
    }
}

impl fmt::Display for Route {
    fn fmt(
        &self,
        formatter: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        formatter.write_str(&self.path)
    }
}

impl From<Route> for String {
    fn from(route: Route) -> Self {
        route.path
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_repo_scope_nests_under_its_project_and_home_has_its_own_mount() {
        let cases = [
            (Route::scope("p", "r"), "/v0/projects/p/repos/r"),
            (Route::scope("p", ""), "/v0/projects/p/home"),
            (Route::chats("p", "r"), "/v0/projects/p/repos/r/chats"),
            (Route::folders("p", ""), "/v0/projects/p/home/chats/folders"),
            (Route::repos("p"), "/v0/projects/p/repos"),
            (
                Route::browse("/my docs"),
                "/v0/system/browse?path=/my%20docs",
            ),
        ];
        for (route, path) in cases {
            assert_eq!(String::from(route), path);
        }
    }
}
