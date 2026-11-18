use std::time::Duration;

use browser::AgentAnswer;
use browseragent::Expectation;

pub(crate) fn settled(wait: Expectation) -> AgentAnswer {
    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .unwrap();
    runtime.block_on(wait.wait(Duration::from_millis(5)))
}
