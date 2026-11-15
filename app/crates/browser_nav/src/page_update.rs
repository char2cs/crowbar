use serde::{Deserialize, Serialize};

use crate::browser_url::BrowserUrl;

const MAX_TITLE_CHARS: usize = 200;

/// What the web engine reported about a browser pane's page. The shell builds these from the
/// engine's own callbacks, never from anything the page posts.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "page", rename_all = "snake_case")]
#[non_exhaustive]
pub enum PageUpdate {
    /// A navigation to `url` started.
    Loading {
        /// The target.
        url: String,
    },
    /// The page at `url` finished loading.
    Loaded {
        /// The loaded address.
        url: String,
    },
    /// The page set a new title.
    Title {
        /// The title, shortened to a displayable length.
        title: String,
    },
}

impl PageUpdate {
    /// A title update, cut to 200 characters so a page cannot flood the chrome.
    #[must_use]
    pub fn title(title: &str) -> Self {
        Self::Title {
            title: title.chars().take(MAX_TITLE_CHARS).collect(),
        }
    }

    /// The JSON text carried from the web view thread to the shell.
    #[must_use]
    pub fn encode(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }

    /// Reads an encoded update; `None` for anything else, and for an address a browser pane
    /// would not load, so the chrome never displays a scheme it refuses.
    #[must_use]
    pub fn decode(text: &str) -> Option<Self> {
        let update: Self = serde_json::from_str(text).ok()?;
        match &update {
            Self::Loading { url } | Self::Loaded { url } => {
                BrowserUrl::is_loadable(url).then_some(update)
            },
            Self::Title { .. } => Some(update),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_update_survives_the_trip() {
        let cases = [
            PageUpdate::Loading {
                url: "https://a.test/".into(),
            },
            PageUpdate::Loaded {
                url: "about:blank".into(),
            },
            PageUpdate::title("Hello"),
        ];
        for update in cases {
            assert_eq!(PageUpdate::decode(&update.encode()), Some(update));
        }
    }

    #[test]
    fn foreign_text_and_unloadable_addresses_are_dropped() {
        let cases = [
            r#"{"sdk":1,"kind":"command.run","command":"x/y"}"#,
            r#"{"page":"loaded","url":"file:///etc/passwd"}"#,
            r#"{"page":"loading","url":"crowbar://core/x"}"#,
            "not json",
        ];
        for text in cases {
            assert_eq!(PageUpdate::decode(text), None, "{text}");
        }
    }

    #[test]
    fn a_long_title_is_cut() {
        let PageUpdate::Title { title } = PageUpdate::title(&"x".repeat(1000)) else {
            panic!("a title update");
        };
        assert_eq!(title.chars().count(), 200);
    }
}
