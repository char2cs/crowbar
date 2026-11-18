//! One request to a daemon route.

use serde_json::Value;

/// One request to a daemon route: its method, path, body and headers.
///
/// Built with a verb such as [`Call::get`], then the body and header builders.
#[derive(Debug, Clone, PartialEq)]
pub struct Call {
    /// The HTTP method, such as `GET`.
    pub method: &'static str,
    /// The route's path, with its query string.
    pub path: String,
    /// The JSON body, when the call sends one.
    pub body: Option<Value>,
    pub(crate) raw: Option<Vec<u8>>,
    /// The extra request headers, as name and value pairs.
    pub headers: Vec<(String, String)>,
}

impl Call {
    fn with(
        method: &'static str,
        path: impl Into<String>,
    ) -> Self {
        Self {
            method,
            path: path.into(),
            body: None,
            raw: None,
            headers: Vec::new(),
        }
    }

    /// A `GET` of `path`, with no body and no extra headers.
    pub fn get(path: impl Into<String>) -> Self {
        Self::with("GET", path)
    }

    /// A `POST` to `path`, with no body and no extra headers.
    pub fn post(path: impl Into<String>) -> Self {
        Self::with("POST", path)
    }

    /// A `PATCH` of `path`, with no body and no extra headers.
    pub fn patch(path: impl Into<String>) -> Self {
        Self::with("PATCH", path)
    }

    /// A `PUT` to `path`, with no body and no extra headers.
    pub fn put(path: impl Into<String>) -> Self {
        Self::with("PUT", path)
    }

    /// A `DELETE` of `path`, with no body and no extra headers.
    pub fn delete(path: impl Into<String>) -> Self {
        Self::with("DELETE", path)
    }

    /// The call with `body` as its JSON body.
    #[must_use]
    pub fn json(
        mut self,
        body: Value,
    ) -> Self {
        self.body = Some(body);
        self
    }

    /// The call with `text` as its body, sent as `application/yaml`.
    #[must_use]
    pub fn yaml(
        mut self,
        text: Vec<u8>,
    ) -> Self {
        self.raw = Some(text);
        self.headers
            .push(("content-type".into(), "application/yaml".into()));
        self
    }

    /// The call with `body` as a `multipart/form-data` body split by `boundary`.
    #[must_use]
    pub fn form(
        mut self,
        body: Vec<u8>,
        boundary: &str,
    ) -> Self {
        self.raw = Some(body);
        self.headers.push((
            "content-type".into(),
            format!("multipart/form-data; boundary={boundary}"),
        ));
        self
    }

    /// The call with `key` as its idempotency key, so a retried call takes effect once.
    #[must_use]
    pub fn idempotent(
        mut self,
        key: &str,
    ) -> Self {
        self.headers.push(("idempotency-key".into(), key.into()));
        self
    }

    /// The call with the `crowbar-discard-work` header, consenting to lose unsaved work.
    #[must_use]
    pub fn discarding_work(mut self) -> Self {
        self.headers
            .push(("crowbar-discard-work".into(), "true".into()));
        self
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn verbs_set_the_method_and_builders_add_body_and_headers() {
        let call = Call::delete("/v0/x")
            .json(json!({"a": 1}))
            .idempotent("k")
            .discarding_work();
        assert_eq!(call.method, "DELETE");
        assert_eq!(call.path, "/v0/x");
        assert_eq!(call.body, Some(json!({"a": 1})));
        assert_eq!(call.headers.len(), 2);
        for (call, method) in [
            (Call::get("/"), "GET"),
            (Call::post("/"), "POST"),
            (Call::patch("/"), "PATCH"),
            (Call::put("/"), "PUT"),
            (Call::delete("/"), "DELETE"),
        ] {
            assert_eq!(call.method, method);
        }
    }
}
