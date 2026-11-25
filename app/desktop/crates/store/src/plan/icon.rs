//! Setting, resetting and uploading the icon of a space.

use std::path::Path;

use daemonapi::Call;
use serde_json::json;
use sidebar::{IconTarget, ProjectId, RowId, SpaceIcon};

use crate::live::Files;
use crate::live::PlanError;
use crate::live::State;
use crate::plan::Plan;
use crate::plan::edit::locate;

const PNG: &str = "image/png";
const JPEG: &str = "image/jpeg";
const WEBP: &str = "image/webp";
pub(crate) const GITHUB_HELP: &str =
    "Check that the repo has a GitHub origin remote and the gh CLI is installed and authenticated.";

fn mime_of(path: &str) -> Result<&'static str, PlanError> {
    let extension = Path::new(path)
        .extension()
        .and_then(std::ffi::OsStr::to_str)
        .map(str::to_ascii_lowercase);
    match extension.as_deref() {
        Some("png") => Ok(PNG),
        Some("jpg" | "jpeg") => Ok(JPEG),
        Some("webp") => Ok(WEBP),
        _ => Err(PlanError::IconType),
    }
}

fn upload(
    files: &dyn Files,
    base: &str,
    path: &str,
) -> Result<Call, PlanError> {
    let mime = mime_of(path)?;
    let bytes = files
        .read(Path::new(path))
        .map_err(PlanError::IconUnreadable)?;
    let name = Path::new(path)
        .file_name()
        .and_then(std::ffi::OsStr::to_str)
        .unwrap_or("icon");
    Ok(Call::put(base.to_owned()).file("icon", (name, mime), &bytes))
}

pub(crate) fn calls(
    files: &dyn Files,
    entity: &str,
    icon: &SpaceIcon,
) -> Result<Vec<Call>, PlanError> {
    let base = format!("{entity}/icon");
    let call = match icon {
        SpaceIcon::Default => Call::delete(base),
        SpaceIcon::Emoji(emoji) => Call::put(format!("{base}/emoji")).json(json!({"emoji": emoji})),
        SpaceIcon::Image(path) => upload(files, &base, path)?,
    };
    Ok(vec![call])
}

pub(crate) fn space_route(project: &str) -> String {
    daemonapi::Route::project(project).to_string()
}

pub(crate) fn space(
    (state, files): (&State, &dyn Files),
    project: &ProjectId,
    icon: &SpaceIcon,
) -> Result<Plan, PlanError> {
    let slot = crate::reads::host_of(state, project.as_str()).ok_or(PlanError::SpaceNotLoaded)?;
    let calls = calls(files, &space_route(project.as_str()), icon)?;
    Ok(Plan::of(&slot.spec.id, calls).marking(IconTarget::Space(project.clone())))
}

fn repo_route(
    state: &State,
    row: &str,
) -> Result<(String, String), PlanError> {
    let (target, host) = locate(state, row)?;
    let route = daemonapi::Route::scope(&target.project, &target.repo).to_string();
    Ok((route, host))
}

pub(crate) fn repo(
    (state, files): (&State, &dyn Files),
    (row, icon): (&str, &SpaceIcon),
) -> Result<Plan, PlanError> {
    let (route, host) = repo_route(state, row)?;
    let target = IconTarget::Repo(RowId::from(row));
    Ok(Plan::of(&host, calls(files, &route, icon)?).marking(target))
}

pub(crate) fn github(
    state: &State,
    row: &str,
) -> Result<Plan, PlanError> {
    let (route, host) = repo_route(state, row)?;
    let call = Call::put(format!("{route}/icon/github"));
    let target = IconTarget::Repo(RowId::from(row));
    Ok(Plan::of(&host, vec![call])
        .explaining(GITHUB_HELP)
        .marking(target))
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::Command;

    use crate::test_support::FakeFiles;

    #[test]
    fn each_icon_kind_reaches_its_route_and_a_bad_image_is_refused_before_any_call() {
        let files = FakeFiles::default();
        let reset = calls(&files, &space_route("p"), &SpaceIcon::Default).unwrap();
        assert_eq!(
            (reset[0].method, reset[0].path.as_str()),
            ("DELETE", "/v0/projects/p/icon")
        );
        let emoji = calls(&files, &space_route("p"), &SpaceIcon::Emoji("🦀".into())).unwrap();
        assert_eq!(emoji[0].path, "/v0/projects/p/icon/emoji");
        assert_eq!(emoji[0].body, Some(json!({"emoji": "🦀"})));
        assert!(calls(&files, &space_route("p"), &SpaceIcon::Image("a.gif".into())).is_err());
        assert!(
            calls(
                &files,
                &space_route("p"),
                &SpaceIcon::Image("/gone.png".into())
            )
            .is_err()
        );
        files.put("/icons/mark.PNG", b"png");
        let image = calls(
            &files,
            &space_route("p"),
            &SpaceIcon::Image("/icons/mark.PNG".into()),
        )
        .unwrap();
        assert_eq!(
            (image[0].method, image[0].path.as_str()),
            ("PUT", "/v0/projects/p/icon")
        );
        assert!(
            image[0]
                .headers
                .iter()
                .any(|(name, value)| name == "content-type"
                    && value.starts_with("multipart/form-data; boundary="))
        );
    }

    type Summary = (&'static str, String, Option<String>, Option<IconTarget>);

    fn summary(plan: Plan) -> Summary {
        let call = &plan.calls[0];
        (call.method, call.path.clone(), plan.failure, plan.marking)
    }

    #[test]
    fn a_repo_header_sets_its_repos_icon_on_the_repo_routes_under_develops_titles() {
        let (state, _) = crate::test_support::state();
        let files = FakeFiles::default();
        let plan_of =
            |command: Command| crate::plan::planner::plan((&state, &files), &command).ok();
        let set = |icon: SpaceIcon| Command::SetRepoIcon {
            row: RowId::from("rdo"),
            icon,
        };
        let cases = [
            (
                set(SpaceIcon::Emoji("⭐".into())),
                ("PUT", "/v0/projects/p/repos/r/icon/emoji"),
                "Failed to set emoji",
            ),
            (
                set(SpaceIcon::Default),
                ("DELETE", "/v0/projects/p/repos/r/icon"),
                "Failed to reset icon",
            ),
            (
                Command::UseGithubAvatar(RowId::from("rdo")),
                ("PUT", "/v0/projects/p/repos/r/icon/github"),
                "Could not fetch the GitHub avatar",
            ),
        ];
        let repo = Some(IconTarget::Repo(RowId::from("rdo")));
        for (command, (method, path), title) in cases {
            let got = plan_of(command.clone()).map(summary);
            let want = (method, path.into(), Some(title.into()), repo.clone());
            assert_eq!(got, Some(want), "{command:?}");
        }
        let github = plan_of(Command::UseGithubAvatar(RowId::from("rdo")));
        let explained = github.and_then(|plan| plan.explained);
        assert_eq!(explained.as_deref(), Some(GITHUB_HELP));
    }

    #[test]
    fn a_space_icon_marks_its_space_busy_while_it_runs() {
        let (state, _) = crate::test_support::state();
        let files = FakeFiles::default();
        let command = Command::SetSpaceIcon {
            project: ProjectId::from("p"),
            icon: SpaceIcon::Default,
        };
        let plan = crate::plan::planner::plan((&state, &files), &command).ok();
        let marked = plan.and_then(|plan| plan.marking);
        assert_eq!(marked, Some(IconTarget::Space(ProjectId::from("p"))));
    }
}
