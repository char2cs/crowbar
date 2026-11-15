use serde_json::Value;
use tauri::ipc::{CommandArg, CommandItem, InvokeBody, InvokeError};
use tauri::Runtime;

const COMMAND: &str = "pair_remote_host";

pub(crate) struct PairingRequest {
    pub(super) host_id: String,
    pub(super) endpoint: String,
    pub(super) fingerprint: String,
    pub(super) code: String,
    pub(super) name: String,
}

impl PairingRequest {
    pub(super) fn from_body(body: &Value) -> Result<Self, String> {
        let field = |key: &str| {
            body.get(key)
                .and_then(Value::as_str)
                .map(str::to_owned)
                .ok_or_else(|| format!("command {COMMAND} missing required key {key}"))
        };
        Ok(Self {
            host_id: field("hostId")?,
            endpoint: field("endpoint")?,
            fingerprint: field("fingerprint")?,
            code: field("code")?,
            name: field("name")?,
        })
    }
}

impl<'de, R: Runtime> CommandArg<'de, R> for PairingRequest {
    fn from_command(command: CommandItem<'de, R>) -> Result<Self, InvokeError> {
        let InvokeBody::Json(body) = command.message.payload() else {
            return Err(InvokeError::from(format!(
                "command {COMMAND} expects a JSON body"
            )));
        };
        Self::from_body(body).map_err(InvokeError::from)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn whole() -> Value {
        json!({
            "hostId": "host-1",
            "endpoint": "1.2.3.4:3737",
            "fingerprint": "ab",
            "code": "123456",
            "name": "laptop",
        })
    }

    #[test]
    fn a_body_with_every_key_reads_back_as_sent() {
        let request = PairingRequest::from_body(&whole()).unwrap();

        let read = [
            request.host_id,
            request.endpoint,
            request.fingerprint,
            request.code,
            request.name,
        ];
        assert_eq!(read, ["host-1", "1.2.3.4:3737", "ab", "123456", "laptop"]);
    }

    #[test]
    fn a_missing_or_mistyped_key_is_named() {
        let cases = [
            ("hostId", None),
            ("endpoint", Some(json!(3737))),
            ("fingerprint", None),
            ("code", Some(json!(null))),
            ("name", None),
        ];
        for (key, replacement) in cases {
            let mut body = whole();
            match replacement {
                Some(value) => body[key] = value,
                None => drop(body.as_object_mut().unwrap().remove(key)),
            }

            let err = PairingRequest::from_body(&body).err().expect(key);

            assert!(
                err.ends_with(&format!("missing required key {key}")),
                "{err}"
            );
        }
    }
}
