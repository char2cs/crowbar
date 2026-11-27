use crate::frontend_asset::FrontendAsset;
use std::ffi::{CStr, CString};
use tauri::Url;

type Resolve = Box<dyn Fn(String) -> Option<FrontendAsset> + Send + Sync>;

pub(crate) struct Frontend {
    dev_url: Option<CString>,
    resolve: Resolve,
}

impl Frontend {
    pub(crate) fn new(
        dev_url: Option<Url>,
        resolve: impl Fn(String) -> Option<FrontendAsset> + Send + Sync + 'static,
    ) -> Self {
        let dev_url = dev_url.and_then(|url| CString::new(url.as_str().trim_end_matches('/')).ok());
        Self {
            dev_url,
            resolve: Box::new(resolve),
        }
    }

    pub(crate) fn address(&self) -> usize {
        std::ptr::from_ref(self).expose_provenance()
    }

    pub(crate) fn dev_url(&self) -> Option<&CStr> {
        self.dev_url.as_deref()
    }

    pub(crate) fn resolve(
        &self,
        path: String,
    ) -> Option<FrontendAsset> {
        (self.resolve)(path)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn asset(bytes: &[u8]) -> FrontendAsset {
        FrontendAsset {
            bytes: bytes.to_vec(),
            mime_type: "text/html".into(),
        }
    }

    #[test]
    fn dev_url_drops_the_trailing_slash() {
        const CASES: &[(&str, &str)] = &[
            ("http://localhost:1420/", "http://localhost:1420"),
            ("http://192.168.1.2:1420", "http://192.168.1.2:1420"),
        ];
        for (given, expected) in CASES {
            let url = Url::parse(given).expect("test urls parse");
            let frontend = Frontend::new(Some(url), |_| None);
            assert_eq!(
                frontend.dev_url().map(CStr::to_bytes),
                Some(expected.as_bytes()),
                "dev url for {given}"
            );
        }
    }

    #[test]
    fn address_is_where_the_frontend_lives() {
        let frontend = Frontend::new(None, |_| None);
        assert_eq!(
            frontend.address(),
            std::ptr::from_ref(&frontend).expose_provenance(),
            "Swift hands this address back to find the frontend"
        );
    }

    #[test]
    fn embedded_frontend_has_no_dev_url() {
        let frontend = Frontend::new(None, |_| None);
        assert!(
            frontend.dev_url().is_none(),
            "embedded builds have no dev server"
        );
    }

    #[test]
    fn resolve_asks_the_resolver_with_the_path() {
        let frontend = Frontend::new(None, |path| {
            (path == "/index.html").then(|| asset(b"<html>"))
        });
        let found = frontend.resolve("/index.html".to_owned());
        assert_eq!(
            found.map(|asset| asset.bytes),
            Some(b"<html>".to_vec()),
            "known path"
        );
        assert!(
            frontend.resolve("/missing".to_owned()).is_none(),
            "unknown path"
        );
    }
}
