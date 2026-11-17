use std::time::Duration;

use browser_nav::BrowserUrl;
use serde_json::Value;

use crate::dom_op::DomOp;
use crate::error::Error;

const DEFAULT_WAIT_MS: u64 = 10_000;
const MAX_WAIT_MS: u64 = 30_000;
const PAGE_OP_LIMIT: Duration = Duration::from_secs(15);
const EVAL_LIMIT: Duration = Duration::from_secs(30);
const WAIT_SLACK: Duration = Duration::from_secs(1);
const URL: &str = "url";
const TIMEOUT_MS: &str = "timeoutMs";
const NEW_TAB: &str = "newTab";
const JS: &str = "js";
const ARGS: &str = "args";

/// What an agent asked of a browser tab, checked before anything is run.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum AgentOp {
    /// Opens a tab in the chat's pane, or reuses one, at an address.
    Open {
        /// Where to go, when given.
        url: Option<BrowserUrl>,
        /// Open another tab even when the chat has one.
        new_tab: bool,
    },
    /// Loads an address.
    Navigate {
        /// Where to go.
        url: BrowserUrl,
    },
    /// Closes the tab.
    Close,
    /// One page back.
    Back,
    /// One page forward.
    Forward,
    /// Loads the page again.
    Reload,
    /// Runs JavaScript, the body of an async function, in the page.
    Eval {
        /// The code.
        js: String,
    },
    /// Takes a picture of what the tab shows.
    Screenshot,
    /// An operation the page script does.
    Page {
        /// Which one.
        op: DomOp,
        /// Its arguments, as the daemon sent them.
        args: Value,
    },
}

impl AgentOp {
    /// Reads the operation `op` with `args`.
    ///
    /// # Errors
    /// [`Error::UnknownOp`], [`Error::Argument`] for an argument that is missing or mistyped,
    /// and [`Error::Address`] for an address a browser pane does not load.
    pub fn parse(
        op: &str,
        args: &Value,
    ) -> Result<Self, Error> {
        match op {
            "open" => open_of(args),
            "navigate" => Ok(Self::Navigate {
                url: required_url("navigate", args)?,
            }),
            "close" => Ok(Self::Close),
            "back" => Ok(Self::Back),
            "forward" => Ok(Self::Forward),
            "reload" => Ok(Self::Reload),
            "eval" => eval_of(args),
            "screenshot" => Ok(Self::Screenshot),
            other => dom_of(other, args),
        }
    }

    /// How long the operation may take before the shell gives up on it.
    #[must_use]
    pub fn limit(&self) -> Duration {
        match self {
            Self::Eval { .. } => EVAL_LIMIT,
            Self::Page {
                op: DomOp::Wait,
                args,
            } => wait_ms(args) + WAIT_SLACK,
            _ => PAGE_OP_LIMIT,
        }
    }
}

fn wait_ms(args: &Value) -> Duration {
    let asked = args.get(TIMEOUT_MS).and_then(Value::as_u64);
    Duration::from_millis(asked.unwrap_or(DEFAULT_WAIT_MS).clamp(1, MAX_WAIT_MS))
}

fn dom_of(
    op: &str,
    args: &Value,
) -> Result<AgentOp, Error> {
    match DomOp::of(op) {
        Some(dom) => page_of(dom, args),
        None => Err(Error::UnknownOp(op.to_owned())),
    }
}

fn object_of(
    op: DomOp,
    args: &Value,
) -> Result<serde_json::Map<String, Value>, Error> {
    match args {
        Value::Null => Ok(serde_json::Map::new()),
        Value::Object(fields) => Ok(fields.clone()),
        _ => Err(Error::Argument {
            op: op.name(),
            name: ARGS,
            expected: "an object",
        }),
    }
}

fn page_of(
    op: DomOp,
    args: &Value,
) -> Result<AgentOp, Error> {
    let mut fields = object_of(op, args)?;
    if op == DomOp::Wait {
        let clamped = u64::try_from(wait_ms(args).as_millis()).unwrap_or(MAX_WAIT_MS);
        fields.insert(TIMEOUT_MS.into(), clamped.into());
    }
    Ok(AgentOp::Page {
        op,
        args: Value::Object(fields),
    })
}

fn open_of(args: &Value) -> Result<AgentOp, Error> {
    let url = match args.get(URL) {
        None | Some(Value::Null) => None,
        Some(_) => Some(required_url("open", args)?),
    };
    let new_tab = args.get(NEW_TAB).and_then(Value::as_bool) == Some(true);
    Ok(AgentOp::Open { url, new_tab })
}

fn eval_of(args: &Value) -> Result<AgentOp, Error> {
    let js = args.get(JS).and_then(Value::as_str);
    js.map(|js| AgentOp::Eval { js: js.to_owned() })
        .ok_or(Error::Argument {
            op: "eval",
            name: JS,
            expected: "a string",
        })
}

fn required_url(
    op: &'static str,
    args: &Value,
) -> Result<BrowserUrl, Error> {
    let text = args
        .get("url")
        .and_then(Value::as_str)
        .map(str::trim)
        .ok_or(Error::Argument {
            op,
            name: URL,
            expected: "a string",
        })?;
    BrowserUrl::parse(text).map_err(|source| Error::Address {
        text: text.to_owned(),
        source,
    })
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn url(text: &str) -> BrowserUrl {
        BrowserUrl::parse(text).unwrap()
    }

    #[test]
    fn open_and_navigate_read_their_address_and_flag() {
        let cases = [
            (
                "open",
                json!({}),
                AgentOp::Open {
                    url: None,
                    new_tab: false,
                },
            ),
            (
                "open",
                json!({"url":"http://a.test/x","newTab":true}),
                AgentOp::Open {
                    url: Some(url("http://a.test/x")),
                    new_tab: true,
                },
            ),
            (
                "navigate",
                json!({"url":" https://a.test "}),
                AgentOp::Navigate {
                    url: url("https://a.test"),
                },
            ),
        ];
        for (op, args, expected) in cases {
            assert_eq!(AgentOp::parse(op, &args).unwrap(), expected, "{op} {args}");
        }
    }

    #[test]
    fn the_other_tools_read_into_their_operation() {
        let cases = [
            ("close", json!(null), AgentOp::Close),
            ("back", json!({}), AgentOp::Back),
            ("forward", json!({}), AgentOp::Forward),
            ("reload", json!({}), AgentOp::Reload),
            (
                "eval",
                json!({"js":"return 1"}),
                AgentOp::Eval {
                    js: "return 1".into(),
                },
            ),
            ("screenshot", json!({}), AgentOp::Screenshot),
            (
                "click",
                json!({"ref":"e1"}),
                AgentOp::Page {
                    op: DomOp::Click,
                    args: json!({"ref":"e1"}),
                },
            ),
        ];
        for (op, args, expected) in cases {
            assert_eq!(AgentOp::parse(op, &args).unwrap(), expected, "{op}");
        }
    }

    #[test]
    fn what_a_browser_pane_would_not_load_or_cannot_run_is_refused_with_the_reason() {
        let cases = [
            ("navigate", json!({"url":"file:///etc/passwd"}), "file"),
            (
                "navigate",
                json!({"url":"javascript:alert(1)"}),
                "javascript",
            ),
            ("navigate", json!({"url":"nonsense"}), "nonsense"),
            ("navigate", json!({}), "url must be a string"),
            ("open", json!({"url":3}), "url must be a string"),
            ("eval", json!({}), "js must be a string"),
            ("teleport", json!({}), "teleport"),
            ("click", json!([1]), "args must be an object"),
            ("snapshot", json!("x"), "args must be an object"),
        ];
        for (op, args, mentions) in cases {
            let error = AgentOp::parse(op, &args).unwrap_err().to_string();
            assert!(error.contains(mentions), "{op}: {error}");
        }
    }

    type Wait = (Option<u64>, u64, u64);

    const WAITS: &[Wait] = &[
        (None, 10_000, 11_000),
        (Some(0), 1, 1_001),
        (Some(500), 500, 1_500),
        (Some(99_000), 30_000, 31_000),
    ];

    fn wait_for(asked: Option<u64>) -> AgentOp {
        let args = match asked {
            Some(ms) => json!({"condition":"load","timeoutMs":ms}),
            None => json!({"condition":"load"}),
        };
        AgentOp::parse("wait", &args).unwrap()
    }

    fn sent_timeout(op: &AgentOp) -> Value {
        match op {
            AgentOp::Page { args, .. } => args["timeoutMs"].clone(),
            other => panic!("a page op, not {other:?}"),
        }
    }

    #[test]
    fn a_wait_is_held_to_the_daemons_range_and_given_a_second_to_answer() {
        for (asked, sent, limit) in WAITS {
            let op = wait_for(*asked);
            assert_eq!(sent_timeout(&op), json!(sent), "{asked:?}");
            assert_eq!(op.limit(), Duration::from_millis(*limit), "{asked:?}");
        }
    }

    #[test]
    fn plain_operations_share_one_limit_and_eval_has_a_longer_one() {
        let plain = AgentOp::parse("snapshot", &json!({})).unwrap().limit();
        let eval = AgentOp::parse("eval", &json!({"js":""})).unwrap().limit();
        assert_eq!((plain, eval), (PAGE_OP_LIMIT, EVAL_LIMIT));
    }
}
