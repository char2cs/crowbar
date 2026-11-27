mod document;
mod lsp;
mod query;

use std::fmt;

use crate::route::query::encode;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Route {
    path: String,
}

impl Route {
    fn at(path: String) -> Self {
        Self { path }
    }

    pub fn project(project: &str) -> Self {
        Self::at(format!("/v0/projects/{project}"))
    }

    pub fn repos(project_id: &str) -> Self {
        Self::at(format!("{}/repos", Self::project(project_id)))
    }

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

    pub fn chats(
        project_id: &str,
        repo: &str,
    ) -> Self {
        Self::at(format!("{}/chats", Self::scope(project_id, repo)))
    }

    pub fn folders(
        project_id: &str,
        repo: &str,
    ) -> Self {
        Self::at(format!("{}/folders", Self::chats(project_id, repo)))
    }

    pub fn document_content(
        workspace: &str,
        path: &str,
    ) -> Self {
        Self::at(document::content(workspace, path))
    }

    pub fn host_document_content(path: &str) -> Self {
        Self::at(document::host_content(path))
    }

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
