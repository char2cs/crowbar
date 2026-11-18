//! The daemon's route prefixes for a project home or a repository.

pub mod document;
pub mod query;

/// The route of the project `project`.
pub fn project(project: &str) -> String {
    format!("/v0/projects/{project}")
}

/// The route of the repositories of the project `project_id`.
pub fn repos(project_id: &str) -> String {
    format!("{}/repos", project(project_id))
}

/// The route prefix of the repository `repo` in the project `project_id`.
///
/// An empty `repo` names the project home instead.
pub fn scope(
    project_id: &str,
    repo: &str,
) -> String {
    let base = project(project_id);
    if repo.is_empty() {
        return format!("{base}/home");
    }
    format!("{base}/repos/{repo}")
}

/// The route of the chats of a repository or project home, as [`scope`] names it.
pub fn chats(
    project_id: &str,
    repo: &str,
) -> String {
    format!("{}/chats", scope(project_id, repo))
}

/// The route of the chat folders of a repository or project home, as [`scope`] names it.
pub fn folders(
    project_id: &str,
    repo: &str,
) -> String {
    format!("{}/folders", chats(project_id, repo))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_repo_scope_nests_under_its_project_and_home_has_its_own_mount() {
        assert_eq!(scope("p", "r"), "/v0/projects/p/repos/r");
        assert_eq!(scope("p", ""), "/v0/projects/p/home");
        assert_eq!(chats("p", "r"), "/v0/projects/p/repos/r/chats");
        assert_eq!(folders("p", ""), "/v0/projects/p/home/chats/folders");
    }
}
