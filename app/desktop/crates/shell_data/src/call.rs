//! One request to a daemon route.

use serde_json::Value;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct Call {
    pub(crate) method: &'static str,
    pub(crate) path: String,
    pub(crate) body: Option<Value>,
    pub(crate) raw: Option<Vec<u8>>,
    pub(crate) headers: Vec<(String, String)>,
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

    pub(crate) fn get(path: impl Into<String>) -> Self {
        Self::with("GET", path)
    }

    pub(crate) fn post(path: impl Into<String>) -> Self {
        Self::with("POST", path)
    }

    pub(crate) fn patch(path: impl Into<String>) -> Self {
        Self::with("PATCH", path)
    }

    pub(crate) fn put(path: impl Into<String>) -> Self {
        Self::with("PUT", path)
    }

    pub(crate) fn delete(path: impl Into<String>) -> Self {
        Self::with("DELETE", path)
    }

    #[must_use]
    pub(crate) fn json(
        mut self,
        body: Value,
    ) -> Self {
        self.body = Some(body);
        self
    }

    #[must_use]
    pub(crate) fn yaml(
        mut self,
        text: Vec<u8>,
    ) -> Self {
        self.raw = Some(text);
        self.headers
            .push(("content-type".into(), "application/yaml".into()));
        self
    }

    #[must_use]
    pub(crate) fn idempotent(
        mut self,
        key: &str,
    ) -> Self {
        self.headers.push(("idempotency-key".into(), key.into()));
        self
    }

    #[must_use]
    pub(crate) fn discarding_work(mut self) -> Self {
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
