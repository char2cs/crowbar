use serde_json::Value;

use crate::client::multipart::file_field;

#[derive(Debug, Clone, PartialEq)]
pub struct Call {
    pub method: &'static str,
    pub path: String,
    pub body: Option<Value>,
    pub(crate) raw: Option<Vec<u8>>,
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

    pub fn get(path: impl Into<String>) -> Self {
        Self::with("GET", path)
    }

    pub fn post(path: impl Into<String>) -> Self {
        Self::with("POST", path)
    }

    pub fn patch(path: impl Into<String>) -> Self {
        Self::with("PATCH", path)
    }

    pub fn put(path: impl Into<String>) -> Self {
        Self::with("PUT", path)
    }

    pub fn delete(path: impl Into<String>) -> Self {
        Self::with("DELETE", path)
    }

    #[must_use]
    pub fn json(
        mut self,
        body: Value,
    ) -> Self {
        self.body = Some(body);
        self
    }

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

    #[must_use]
    pub fn file(
        mut self,
        field: &str,
        (name, mime): (&str, &str),
        bytes: &[u8],
    ) -> Self {
        let (body, boundary) = file_field(field, (name, mime), bytes);
        self.raw = Some(body);
        self.headers.push((
            "content-type".into(),
            format!("multipart/form-data; boundary={boundary}"),
        ));
        self
    }

    #[must_use]
    pub fn idempotent(
        mut self,
        key: &str,
    ) -> Self {
        self.headers.push(("idempotency-key".into(), key.into()));
        self
    }

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
