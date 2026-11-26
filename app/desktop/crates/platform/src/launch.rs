use std::ffi::OsString;
use std::path::Path;
use std::process::{Command, Stdio};

use crate::desktop::Desktop;
use crate::error::Error;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Launch {
    program: &'static str,
    args: Vec<OsString>,
}

impl Launch {
    #[must_use]
    pub(crate) fn open(
        desktop: Desktop,
        target: impl Into<OsString>,
    ) -> Self {
        Self {
            program: opener(desktop),
            args: vec![target.into()],
        }
    }

    // PLATFORM: macOS selects the item with `open -R`; freedesktop has no portable "select this file", so the containing directory is opened instead.
    #[must_use]
    pub(crate) fn reveal(
        desktop: Desktop,
        path: &Path,
    ) -> Self {
        match desktop {
            Desktop::MacOs => Self::select(path),
            Desktop::Linux => Self::open(desktop, path.parent().unwrap_or(path)),
        }
    }

    #[must_use]
    pub(crate) fn reveal_directory(
        desktop: Desktop,
        path: &Path,
    ) -> Self {
        match desktop {
            Desktop::MacOs => Self::select(path),
            Desktop::Linux => Self::open(desktop, path),
        }
    }

    fn select(path: &Path) -> Self {
        Self {
            program: "open",
            args: vec!["-R".into(), path.into()],
        }
    }

    pub(crate) fn spawn(&self) -> Result<(), Error> {
        let mut child = Command::new(self.program)
            .args(&self.args)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|source| Error::Spawn {
                program: self.program,
                source,
            })?;
        let program = self.program;
        std::thread::spawn(move || reap(&mut child, program));
        Ok(())
    }
}

fn reap(
    child: &mut std::process::Child,
    program: &str,
) {
    if let Err(error) = child.wait() {
        tracing::warn!(%error, program, "launcher could not be reaped");
    }
}

fn opener(desktop: Desktop) -> &'static str {
    match desktop {
        Desktop::MacOs => "open",
        Desktop::Linux => "xdg-open",
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;

    fn args(launch: &Launch) -> Vec<String> {
        launch
            .args
            .iter()
            .map(|arg| arg.to_string_lossy().into_owned())
            .collect()
    }

    #[test]
    fn each_desktop_opens_with_its_opener_and_reveals_in_its_own_way() {
        let file = Path::new("/a/b.txt");
        let cases = [
            (
                Launch::open(Desktop::Linux, file),
                "xdg-open",
                vec!["/a/b.txt"],
            ),
            (Launch::open(Desktop::MacOs, file), "open", vec!["/a/b.txt"]),
            (
                Launch::reveal(Desktop::MacOs, file),
                "open",
                vec!["-R", "/a/b.txt"],
            ),
            (Launch::reveal(Desktop::Linux, file), "xdg-open", vec!["/a"]),
            (
                Launch::reveal_directory(Desktop::Linux, Path::new("/a/b")),
                "xdg-open",
                vec!["/a/b"],
            ),
        ];
        for (launch, program, expected) in cases {
            assert_eq!(
                (launch.program, args(&launch)),
                (program, expected.iter().map(ToString::to_string).collect())
            );
        }
    }

    #[test]
    fn a_missing_launcher_is_an_error_not_a_panic() {
        let launch = Launch {
            program: "crowbar-no-such-launcher",
            args: Vec::new(),
        };
        assert!(matches!(launch.spawn(), Err(Error::Spawn { .. })));
    }
}
