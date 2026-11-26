use std::path::{Path, PathBuf};

use crate::error::{Error, Result};

const DEFAULT_PANES_DIR: &str = "panes";
const PACKAGED_PANES_DIRS: [&str; 2] = ["lib/crowbar/panes", "Resources/panes"];

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct AppOptions {
    pub(crate) panes_dir: Option<PathBuf>,
    pub(crate) chat_entry: String,
    pub(crate) home: PathBuf,
    pub(crate) daemon_binary: Option<PathBuf>,
}

impl AppOptions {
    pub(crate) fn from_env(vars: impl Fn(&str) -> Option<String>) -> Self {
        let home = vars("CROWBAR_HOME")
            .filter(|home| !home.is_empty())
            .or_else(|| vars("HOME").map(|home| format!("{home}/.crowbar")))
            .map_or_else(|| PathBuf::from(".crowbar"), PathBuf::from);
        Self {
            panes_dir: vars("CROWBAR_PANES_DIR")
                .filter(|dir| !dir.is_empty())
                .map(PathBuf::from),
            chat_entry: vars("CROWBAR_CHAT_ENTRY").unwrap_or_else(|| "chat/index.html".into()),
            home,
            daemon_binary: vars("CROWBAR_API_BIN").map(PathBuf::from),
        }
    }

    pub(crate) fn resolve_panes_dir(
        &self,
        exe: Option<&Path>,
        is_file: impl Fn(&Path) -> bool,
    ) -> Result<PathBuf> {
        let candidates = match (&self.panes_dir, exe.and_then(Path::parent)) {
            (Some(dir), _) => {
                vec![std::path::absolute(dir).map_err(|source| Error::Read(dir.clone(), source))?]
            },
            (None, Some(exe_dir)) => installed_panes_dirs(exe_dir),
            (None, None) => vec![PathBuf::from(DEFAULT_PANES_DIR)],
        };
        let entry_of = |dir: &PathBuf| dir.join(&self.chat_entry);
        let first_entry = candidates.first().map(entry_of).unwrap_or_default();
        candidates
            .into_iter()
            .find(|dir| is_file(&entry_of(dir)))
            .ok_or(Error::PanesMissing(first_entry))
    }

    pub(crate) fn state_dir(&self) -> PathBuf {
        self.home.join("shell")
    }
}

fn installed_panes_dirs(exe_dir: &Path) -> Vec<PathBuf> {
    let packaged = exe_dir
        .parent()
        .into_iter()
        .flat_map(|prefix| PACKAGED_PANES_DIRS.map(|dir| prefix.join(dir)));
    std::iter::once(exe_dir.join(DEFAULT_PANES_DIR))
        .chain(packaged)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    type Expected = (
        &'static str,
        Option<&'static str>,
        &'static str,
        Option<&'static str>,
    );
    type EnvCase = (
        &'static str,
        &'static [(&'static str, &'static str)],
        Expected,
    );

    const ENV_CASES: &[EnvCase] = &[
        (
            "defaults follow the home directory",
            &[("HOME", "/home/u")],
            ("/home/u/.crowbar", None, "chat/index.html", None),
        ),
        (
            "the environment overrides everything",
            &[
                ("CROWBAR_HOME", "/h"),
                ("CROWBAR_PANES_DIR", "/p"),
                ("CROWBAR_CHAT_ENTRY", "index.html"),
                ("CROWBAR_API_BIN", "/bin/api"),
            ],
            ("/h", Some("/p"), "index.html", Some("/bin/api")),
        ),
        (
            "an empty home variable counts as unset",
            &[("CROWBAR_HOME", ""), ("HOME", "/home/u")],
            ("/home/u/.crowbar", None, "chat/index.html", None),
        ),
        (
            "an empty panes directory counts as unset",
            &[("CROWBAR_PANES_DIR", "")],
            (".crowbar", None, "chat/index.html", None),
        ),
    ];

    fn env_of(vars: &[(&str, &str)]) -> impl Fn(&str) -> Option<String> {
        let vars: Vec<(String, String)> = vars
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect();
        move |key| {
            vars.iter()
                .find(|(known, _)| known == key)
                .map(|(_, value)| value.clone())
        }
    }

    fn options_of(expected: &Expected) -> AppOptions {
        let (home, panes, entry, binary) = expected;
        AppOptions {
            panes_dir: panes.map(PathBuf::from),
            chat_entry: (*entry).to_owned(),
            home: PathBuf::from(home),
            daemon_binary: binary.map(PathBuf::from),
        }
    }

    #[test]
    fn the_environment_decides_the_options() {
        for (name, vars, expected) in ENV_CASES {
            let options = AppOptions::from_env(env_of(vars));
            let expected = options_of(expected);
            assert_eq!(options, expected, "{name}");
            assert_eq!(options.state_dir(), expected.home.join("shell"), "{name}");
        }
    }

    fn options_with(panes: Option<PathBuf>) -> AppOptions {
        AppOptions {
            panes_dir: panes,
            chat_entry: "chat/index.html".into(),
            home: PathBuf::from("/h"),
            daemon_binary: None,
        }
    }

    type ResolveCase = (
        &'static str,
        Option<&'static str>,
        Option<&'static str>,
        Option<&'static str>,
        Option<&'static str>,
    );

    const RESOLVE_CASES: &[ResolveCase] = &[
        (
            "the panes default to a directory beside the executable",
            None,
            Some("crowbar"),
            Some("panes"),
            Some("panes"),
        ),
        (
            "a deb or AppImage keeps the panes under lib beside bin",
            None,
            Some("usr/bin/crowbar"),
            Some("usr/lib/crowbar/panes"),
            Some("usr/lib/crowbar/panes"),
        ),
        (
            "a macOS bundle keeps the panes in its resources",
            None,
            Some("Crowbar.app/Contents/MacOS/crowbar"),
            Some("Crowbar.app/Contents/Resources/panes"),
            Some("Crowbar.app/Contents/Resources/panes"),
        ),
        (
            "an explicit directory wins",
            Some("explicit"),
            None,
            Some("explicit"),
            Some("explicit"),
        ),
        (
            "a missing bundle is an error naming the entry page",
            Some("empty"),
            None,
            None,
            None,
        ),
    ];

    fn check_resolved(
        name: &str,
        found: Result<PathBuf>,
        expected: Option<PathBuf>,
    ) {
        let Some(expected) = expected else {
            let error = found.unwrap_err();
            assert!(
                error.to_string().contains("chat/index.html"),
                "{name}: {error}"
            );
            return;
        };
        let dir = found.unwrap();
        assert_eq!(dir, expected, "{name}");
        assert!(dir.is_absolute(), "{name}: absolute");
    }

    const ROOT: &str = "/root";

    fn run_case(case: &ResolveCase) {
        let (name, configured, exe, bundled, resolved) = case;
        let at = |name: &str| Path::new(ROOT).join(name);
        let entry = bundled.map(|dir| at(dir).join("chat/index.html"));
        let options = options_with(configured.map(at));
        let found = options.resolve_panes_dir(exe.map(at).as_deref(), |file| {
            entry.as_deref() == Some(file)
        });
        check_resolved(name, found, resolved.map(at));
    }

    #[test]
    fn the_panes_directory_is_found_or_the_missing_entry_is_named() {
        RESOLVE_CASES.iter().for_each(run_case);
    }
}
