use std::time::Duration;

use browser_agent::Expectation;
use browser_nav::AgentAnswer;

pub(crate) fn settled(wait: Expectation) -> AgentAnswer {
    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .unwrap();
    runtime.block_on(wait.wait(Duration::from_millis(5)))
}
