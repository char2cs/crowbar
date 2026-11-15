use protocol::PaneOrigin;

use crate::route::Route;

pub(crate) fn allows(origin: &PaneOrigin, route: &Route) -> bool {
    match (origin, route) {
        (PaneOrigin::Core, Route::CoreBundle { .. } | Route::Daemon { .. }) => true,
        (PaneOrigin::Extension(own), Route::ExtensionBundle { extension, .. }) => own == extension,
        (PaneOrigin::Extension(own), Route::Daemon { path_and_query, .. }) => {
            path_and_query.starts_with(&format!("/v0/ext/{own}/"))
        },
        (PaneOrigin::Core, Route::ExtensionBundle { .. })
        | (PaneOrigin::Extension(_), Route::CoreBundle { .. }) => false,
    }
}

#[cfg(test)]
#[path = "policy_tests.rs"]
mod tests;
