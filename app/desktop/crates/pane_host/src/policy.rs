use protocol::PaneKind;

use crate::origin::Origin;
use crate::route::Route;

pub(crate) fn allows(origin: &Origin, route: &Route) -> bool {
    match (origin.kind, route) {
        (PaneKind::Core, Route::CoreBundle { .. } | Route::Daemon { .. }) => true,
        (PaneKind::Core, Route::ExtensionBundle { .. }) => false,
        (PaneKind::Extension, Route::ExtensionBundle { extension, .. }) => {
            origin.extension.as_ref() == Some(extension)
        },
        (PaneKind::Extension, Route::Daemon { path_and_query, .. }) => origin
            .extension
            .as_ref()
            .is_some_and(|own| path_and_query.starts_with(&format!("/v0/ext/{own}/"))),
        (PaneKind::Extension, Route::CoreBundle { .. }) => false,
    }
}

#[cfg(test)]
#[path = "policy_tests.rs"]
mod tests;
