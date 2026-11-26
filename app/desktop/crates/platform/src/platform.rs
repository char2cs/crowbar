use std::path::Path;

use crate::desktop::Desktop;
use crate::error::Error;
use crate::launch::Launch;
use crate::link_target::{LinkTarget, classify};

/// Opens an `http(s)`, `mailto` or `tel` link in the system's default application.
///
/// # Errors
/// [`Error::Refused`] for any other link, [`Error::Spawn`] when the launcher cannot start.
pub fn open_external(link: &str) -> Result<(), Error> {
    if classify(link, None) != LinkTarget::SystemBrowser {
        return Err(refused(link));
    }
    Launch::open(Desktop::current(), link).spawn()
}

fn refused(link: &str) -> Error {
    Error::Refused {
        target: link.to_owned(),
        reason: "only web, mail and phone links leave the app",
    }
}

fn hand_to_browser(link: &str) {
    if let Err(error) = open_external(link) {
        tracing::warn!(%error, "a link could not be opened in the system browser");
    }
}

/// Decides a navigation request of a pane's web view: `true` lets the view load `link`;
/// a web, mail or phone link is handed to the system browser instead and the view stays put,
/// and anything else is refused.
///
/// `crowbar:` and `about:` links are the app's own, and so is `dev_origin`, the dev server of a
/// development build. `data:`, `blob:`, `file:` and `javascript:` are never followed: a document
/// of either of the first two has an opaque origin and must not replace the app.
pub fn follow_link(
    link: &str,
    dev_origin: Option<&str>,
) -> bool {
    match classify(link, dev_origin) {
        LinkTarget::InApp => true,
        LinkTarget::SystemBrowser => {
            hand_to_browser(link);
            false
        },
        LinkTarget::Refuse => false,
    }
}

/// Opens an existing file or directory with the default application, for example the log file.
///
/// # Errors
/// [`Error::Unresolved`] when the path does not exist, [`Error::Spawn`] when the launcher cannot start.
pub fn open_path(path: &Path) -> Result<(), Error> {
    let resolved = resolve(path)?;
    Launch::open(Desktop::current(), resolved).spawn()
}

/// Shows an existing path in the file manager.
///
/// # Errors
/// [`Error::Unresolved`] when the path does not exist, [`Error::Spawn`] when the launcher cannot start.
pub fn reveal_in_file_manager(path: &Path) -> Result<(), Error> {
    let resolved = resolve(path)?;
    if resolved.is_dir() {
        return Launch::reveal_directory(Desktop::current(), &resolved).spawn();
    }
    Launch::reveal(Desktop::current(), &resolved).spawn()
}

fn resolve(path: &Path) -> Result<std::path::PathBuf, Error> {
    path.canonicalize().map_err(|source| Error::Unresolved {
        path: path.to_path_buf(),
        source,
    })
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;

    const CASES: &[(&str, bool)] = &[
        ("crowbar://core/panes/core/chat/index.html", true),
        ("file:///etc/passwd", false),
        ("javascript:alert(1)", false),
        ("data:text/html,x", false),
        ("--help", false),
        ("", false),
    ];

    #[test]
    fn a_view_keeps_app_pages_and_only_web_mail_and_phone_links_leave_the_app() {
        for (link, followed) in CASES {
            assert_eq!(follow_link(link, None), *followed, "{link}");
            assert!(
                matches!(open_external(link), Err(Error::Refused { .. })),
                "{link} does not leave"
            );
        }
    }

    #[test]
    fn a_missing_path_is_reported_before_any_launcher_runs() {
        let missing = Path::new("/definitely/not/here/crowbar");
        assert!(matches!(
            reveal_in_file_manager(missing),
            Err(Error::Unresolved { .. })
        ));
        assert!(matches!(open_path(missing), Err(Error::Unresolved { .. })));
    }
}
