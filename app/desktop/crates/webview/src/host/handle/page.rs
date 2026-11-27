use crate::error::Result;
use crate::host::handle::{Handle, RECEIVER};
use crate::view::Snapshot;

impl Handle {
    pub fn eval(
        &self,
        script: &str,
    ) -> Result<()> {
        self.platform.eval(self.id, script)
    }

    pub fn navigate(
        &self,
        url: &str,
    ) -> Result<()> {
        self.platform.navigate(self.id, url)
    }

    pub fn snapshot(&self) -> Result<Option<Snapshot>> {
        self.platform.snapshot(self.id)
    }

    pub fn post_message(
        &self,
        text: &str,
    ) -> Result<()> {
        self.eval(&receive_script(text))
    }
}

fn receive_script(text: &str) -> String {
    let literal = serde_json::to_string(text).unwrap_or_else(|_| String::from("\"\""));
    format!("window.{RECEIVER}&&window.{RECEIVER}({literal})")
}

#[cfg(test)]
mod tests {
    use crate::backend::Call;
    use crate::test_support::view_fixtures::handle;

    const CASES: &[(&str, &str, &str)] = &[
        (
            "quotes, newlines and a closing script tag",
            "a\"b\n</script>",
            r#"window.__crowbarReceive&&window.__crowbarReceive("a\"b\n</script>")"#,
        ),
        (
            "plain text",
            "hi",
            r#"window.__crowbarReceive&&window.__crowbarReceive("hi")"#,
        ),
    ];

    #[test]
    fn post_message_escapes_the_text_into_a_script_literal() {
        for (name, text, expected) in CASES {
            let (mock, view) = handle();
            view.post_message(text).unwrap();
            let script = mock.calls().into_iter().find_map(|call| match call {
                Call::Eval { script, .. } => Some(script),
                _ => None,
            });
            assert_eq!(script.as_deref(), Some(*expected), "{name}");
        }
    }
}
