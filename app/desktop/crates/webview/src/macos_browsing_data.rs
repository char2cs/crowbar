use std::ptr::NonNull;
use std::rc::Rc;

use platform::AppkitParent;
use wry::{WebView, WebViewBuilder, WebViewBuilderExtDarwin};

use crate::browsing_data::BrowsingData;
use crate::error::{Error, Result};
use crate::macos_data_store::BROWSER_STORE;
use crate::macos_platform::MacosPlatform;
use crate::web_view_kind::WebViewKind;

fn scratch_browser_view(address: Option<usize>) -> Result<WebView> {
    let address = address.ok_or(Error::UnsupportedParent("no window hosts a web view yet"))?;
    let view = NonNull::new(std::ptr::with_exposed_provenance_mut(address))
        .ok_or(Error::UnsupportedParent("null NSView"))?;
    Ok(WebViewBuilder::new()
        .with_data_store_identifier(BROWSER_STORE)
        .build_as_child(&AppkitParent::new(view))?)
}

impl BrowsingData for MacosPlatform {
    fn clear_browsing_data(&self) -> Result<()> {
        let open: Option<Rc<WebView>> = {
            let kinds = self.kinds.borrow();
            let views = self.views.borrow();
            kinds
                .iter()
                .find(|(_, kind)| **kind == WebViewKind::Browser)
                .and_then(|(id, _)| views.get(id).cloned())
        };
        if let Some(browser) = open {
            return Ok(browser.clear_all_browsing_data()?);
        }
        let scratch = scratch_browser_view(self.last_parent.get())?;
        Ok(scratch.clear_all_browsing_data()?)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_scratch_view_needs_a_live_window_to_sit_in() {
        let refused = [None, Some(0)];
        for address in refused {
            let result = scratch_browser_view(address);
            assert!(
                matches!(result, Err(Error::UnsupportedParent(_))),
                "no window at {address:?}"
            );
        }
    }
}
