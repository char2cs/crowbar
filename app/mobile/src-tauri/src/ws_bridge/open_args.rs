use std::str::FromStr;

use serde_json::Value;
use tauri::ipc::{Channel, InvokeBody, InvokeResponseBody, JavaScriptChannelId, Request};
use tauri::{Runtime, Webview};

pub(super) struct OpenArgs {
    pub(super) conn_id: String,
    pub(super) path: String,
    pub(super) host_id: Option<String>,
    pub(super) on_message: String,
    pub(super) idle_timeout_ms: Option<u64>,
}

fn required(
    args: &Value,
    key: &str,
) -> Result<String, String> {
    args.get(key)
        .and_then(Value::as_str)
        .map(str::to_owned)
        .ok_or_else(|| format!("invalid args `{key}` for command `ws_open`: expected a string"))
}

fn optional_text(
    args: &Value,
    key: &str,
) -> Option<String> {
    args.get(key).and_then(Value::as_str).map(str::to_owned)
}

impl OpenArgs {
    pub(super) fn from_json(args: &Value) -> Result<Self, String> {
        Ok(Self {
            conn_id: required(args, "connId")?,
            path: required(args, "path")?,
            host_id: optional_text(args, "hostId"),
            on_message: required(args, "onMessage")?,
            idle_timeout_ms: args.get("idleTimeoutMs").and_then(Value::as_u64),
        })
    }

    pub(super) fn from_request(request: &Request<'_>) -> Result<Self, String> {
        match request.body() {
            InvokeBody::Json(args) => Self::from_json(args),
            InvokeBody::Raw(_) => Err("ws_open expects JSON arguments".to_owned()),
        }
    }

    pub(super) fn channel<R: Runtime>(
        &self,
        webview: &Webview<R>,
    ) -> Result<Channel<InvokeResponseBody>, String> {
        let id = JavaScriptChannelId::from_str(&self.on_message)
            .map_err(|e| format!("invalid args `onMessage` for command `ws_open`: {e}"))?;
        Ok(id.channel_on(webview.clone()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn the_js_argument_object_maps_onto_the_open_arguments() {
        let full = json!({
            "connId": "c1", "path": "/v0/x", "hostId": "h1",
            "onMessage": "__CHANNEL__:7", "idleTimeoutMs": 30000,
        });
        let args = OpenArgs::from_json(&full).unwrap();
        assert_eq!(args.conn_id, "c1");
        assert_eq!(args.path, "/v0/x");
        assert_eq!(args.host_id.as_deref(), Some("h1"));
        assert_eq!(args.on_message, "__CHANNEL__:7");
        assert_eq!(args.idle_timeout_ms, Some(30000));
    }

    #[test]
    fn optional_arguments_may_be_null_or_absent_and_required_ones_may_not() {
        const CASES: &[(&str, bool)] = &[
            (
                r#"{"connId":"c","path":"/p","onMessage":"m","hostId":null,"idleTimeoutMs":null}"#,
                true,
            ),
            (r#"{"connId":"c","path":"/p","onMessage":"m"}"#, true),
            (r#"{"path":"/p","onMessage":"m"}"#, false),
            (r#"{"connId":"c","onMessage":"m"}"#, false),
            (r#"{"connId":"c","path":"/p"}"#, false),
            (r#"{"connId":1,"path":"/p","onMessage":"m"}"#, false),
        ];
        for (text, accepted) in CASES {
            let value: Value = serde_json::from_str(text).unwrap();
            assert_eq!(OpenArgs::from_json(&value).is_ok(), *accepted, "{text}");
        }
    }
}
