use shell_data::{Tool, ToolReport};
use ui::IconName;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum CheckStatus {
    Checking,
    Ready,
    Optional,
    Missing,
}

impl CheckStatus {
    pub(crate) fn of(
        tool: Tool,
        report: Option<&ToolReport>,
    ) -> Self {
        let Some(report) = report else {
            return Self::Checking;
        };
        match (tool, report) {
            (Tool::Git, report) if report.installed => Self::Ready,
            (Tool::Git, _) => Self::Missing,
            (_, report) if report.authed => Self::Ready,
            _ => Self::Optional,
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            Self::Checking => IconName::CircleNotch,
            Self::Ready => IconName::CheckCircle,
            Self::Optional => IconName::Warning,
            Self::Missing => IconName::XCircle,
        }
    }
}

pub(crate) fn label(tool: Tool) -> &'static str {
    match tool {
        Tool::Git => "Git",
        Tool::Gh => "GitHub CLI (gh)",
        Tool::Glab => "GitLab CLI (glab)",
    }
}

fn cli_command(tool: Tool) -> &'static str {
    match tool {
        Tool::Glab => "glab",
        _ => "gh",
    }
}

fn forge_name(tool: Tool) -> &'static str {
    match tool {
        Tool::Glab => "GitLab",
        _ => "GitHub",
    }
}

pub(crate) fn description(
    tool: Tool,
    report: Option<&ToolReport>,
) -> String {
    let Some(report) = report else {
        return "Checking\u{2026}".to_owned();
    };
    if tool == Tool::Git {
        return git_description(report);
    }
    if report.authed {
        return "Authenticated".to_owned();
    }
    if report.installed {
        return format!(
            "Not signed in \u{2014} run: {} auth login",
            cli_command(tool)
        );
    }
    format!(
        "Not installed \u{2014} optional, enables {} features",
        forge_name(tool)
    )
}

fn git_description(report: &ToolReport) -> String {
    if !report.installed {
        return "Not found \u{2014} install from git-scm.com".to_owned();
    }
    report
        .version
        .clone()
        .unwrap_or_else(|| "Installed".to_owned())
}

#[cfg(test)]
mod tests {
    use super::*;

    const ABSENT: ToolReport = ToolReport {
        installed: false,
        authed: false,
        version: None,
    };
    const INSTALLED: ToolReport = ToolReport {
        installed: true,
        authed: false,
        version: None,
    };
    const SIGNED_IN: ToolReport = ToolReport {
        installed: true,
        authed: true,
        version: None,
    };

    fn git_version() -> ToolReport {
        ToolReport {
            installed: true,
            authed: false,
            version: Some("git version 2".into()),
        }
    }

    #[test]
    fn git_is_required_and_a_forge_cli_is_only_optional() {
        let cases = [
            (Tool::Git, None, CheckStatus::Checking),
            (Tool::Git, Some(git_version()), CheckStatus::Ready),
            (Tool::Git, Some(ABSENT), CheckStatus::Missing),
            (Tool::Gh, Some(SIGNED_IN), CheckStatus::Ready),
            (Tool::Gh, Some(INSTALLED), CheckStatus::Optional),
            (Tool::Glab, Some(ABSENT), CheckStatus::Optional),
        ];
        for (tool, report, expected) in cases {
            assert_eq!(CheckStatus::of(tool, report.as_ref()), expected, "{tool:?}");
        }
    }

    #[test]
    fn each_row_explains_itself_in_the_words_of_the_old_screen() {
        let cases = [
            (Tool::Git, None, "Checking\u{2026}"),
            (Tool::Git, Some(git_version()), "git version 2"),
            (Tool::Git, Some(INSTALLED), "Installed"),
            (
                Tool::Git,
                Some(ABSENT),
                "Not found \u{2014} install from git-scm.com",
            ),
            (Tool::Gh, Some(SIGNED_IN), "Authenticated"),
            (
                Tool::Gh,
                Some(INSTALLED),
                "Not signed in \u{2014} run: gh auth login",
            ),
            (
                Tool::Glab,
                Some(INSTALLED),
                "Not signed in \u{2014} run: glab auth login",
            ),
            (
                Tool::Glab,
                Some(ABSENT),
                "Not installed \u{2014} optional, enables GitLab features",
            ),
            (
                Tool::Gh,
                Some(ABSENT),
                "Not installed \u{2014} optional, enables GitHub features",
            ),
        ];
        for (tool, report, expected) in cases {
            assert_eq!(description(tool, report.as_ref()), expected, "{tool:?}");
        }
    }
}
