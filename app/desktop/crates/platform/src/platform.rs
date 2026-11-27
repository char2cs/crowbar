use std::path::Path;

use crate::desktop::Desktop;
use crate::error::Error;
use crate::launch::Launch;
use crate::link_target::{LinkTarget, classify};

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

pub fn open_path(path: &Path) -> Result<(), Error> {
    let resolved = resolve(path)?;
    Launch::open(Desktop::current(), resolved).spawn()
}

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

    #[test]
    fn only_web_mail_and_phone_links_may_leave_the_app() {
        const CASES: &[&str] = &[
            "file:///etc/passwd",
            "javascript:alert(1)",
            "crowbar://x",
            "--help",
            "",
        ];
        for link in CASES {
            assert!(
                matches!(open_external(link), Err(Error::Refused { .. })),
                "{link}"
            );
        }
    }

    #[test]
    fn a_view_keeps_app_pages_and_never_follows_refused_links() {
        const CASES: &[(&str, bool)] = &[
            ("crowbar://core/panes/core/chat/index.html", true),
            ("file:///etc/passwd", false),
            ("javascript:alert(1)", false),
            ("data:text/html,x", false),
        ];
        for (link, followed) in CASES {
            assert_eq!(follow_link(link, None), *followed, "{link}");
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
