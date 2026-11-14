use std::ffi::{CStr, CString};
use std::sync::OnceLock;
use crate::frontend_asset::FrontendAsset;
use tauri::Url;

type Resolve = Box<dyn Fn(String) -> Option<FrontendAsset> + Send + Sync>;

pub(crate) struct Frontend {
    dev_url: Option<CString>,
    resolve: Resolve,
}

// PLATFORM: Swift reaches the frontend through context-free C functions, so the installed
// PLATFORM: frontend is process-wide.
static FRONTEND: OnceLock<Frontend> = OnceLock::new();

impl Frontend {
    pub(crate) fn new(
        dev_url: Option<Url>,
        resolve: impl Fn(String) -> Option<FrontendAsset> + Send + Sync + 'static,
    ) -> Self {
        let dev_url = dev_url
            .and_then(|url| CString::new(url.as_str().trim_end_matches('/')).ok());
        Self {
            dev_url,
            resolve: Box::new(resolve),
        }
    }

    pub(crate) fn install(
        dev_url: Option<Url>,
        resolve: impl Fn(String) -> Option<FrontendAsset> + Send + Sync + 'static,
    ) {
        FRONTEND.get_or_init(|| Self::new(dev_url, resolve));
    }

    pub(crate) fn installed() -> Option<&'static Self> {
        FRONTEND.get()
    }

    pub(crate) fn dev_url(&self) -> Option<&CStr> {
        self.dev_url.as_deref()
    }

    pub(crate) fn resolve(&self, path: String) -> Option<FrontendAsset> {
        (self.resolve)(path)
    }
}

#[cfg(test)]
#[path = "frontend_tests.rs"]
mod tests;
