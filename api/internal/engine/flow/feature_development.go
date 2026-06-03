package flow

// FeatureDevelopmentFlow is the hardcoded built-in feature development flow.
// It is returned directly by the Loader when flowPath is "" or
// "builtin:feature-development" — no disk I/O, no parsing, always valid.
var FeatureDevelopmentFlow = FlowDefinition{
	Name:         "feature-development",
	Version:      "1.0",
	Description:  "Full feature development — brainstorm to reviewed implementation",
	ItemStatuses: []string{"todo", "implementing", "done"},
	States: []StateDefinition{
		{
			Name: "brainstorming",
			UI:   UIModeChat,
			Agent: &AgentDef{
				Intelligence: IntelligenceMedium,
				Tools:        []FlowTool{ToolCrowbarSignal, ToolFSRead, ToolTerminal},
				SystemPrompt: `You are a senior engineer partnering with the developer to explore
a feature idea. Ask clarifying questions, explore edge cases, and
help arrive at a clear implementation intent.
When aligned, call crowbar_signal("user_approved").
If a bug is uncovered instead, call crowbar_signal("bug_identified").`,
			},
			Transitions: []TransitionDef{
				{To: "spec", On: "user_approved"},
				{To: "debugging", On: "bug_identified"},
			},
		},
		{
			Name: "spec",
			UI:   UIModeChat,
			Agent: &AgentDef{
				Intelligence: IntelligenceMedium,
				Tools:        []FlowTool{ToolCrowbarSignal, ToolFSRead},
				SystemPrompt: `Based on the brainstorming conversation, produce a structured spec
with acceptance criteria and a list of implementation tasks.
Present it to the developer. On approval, call crowbar_signal("user_approved").
If they want to revisit, call crowbar_signal("revision_requested").`,
			},
			Transitions: []TransitionDef{
				{To: "implementation", On: "user_approved"},
				{To: "brainstorming", On: "revision_requested"},
			},
		},
		{
			Name:  "implementation",
			UI:    UIModeKanban,
			Items: true,
			Agent: &AgentDef{
				Intelligence: IntelligenceHigh,
				Tools: []FlowTool{
					ToolCrowbarSignal,
					ToolCrowbarCreateItem,
					ToolCrowbarUpdateItemStatus,
					ToolCrowbarGetItems,
					ToolCrowbarGetThreads,
					ToolCrowbarReplyThread,
					ToolFSRead,
					ToolFSWrite,
					ToolTerminal,
				},
				SystemPrompt: `You may be entering this state for the first time (fresh implementation) or
returning from AI review with open threads to address.

On first entry: create an item for each task, implement them one at a time,
marking each crowbar_update_item_status(item_id, "implementing") when starting
and crowbar_update_item_status(item_id, "done") when complete.

On re-entry after review: check open AI review threads with
crowbar_get_threads(status="open", phase="ai_review"). Address each one, make
the fix, and reply documenting what you changed using crowbar_reply_thread.

When all items are "done" and no open threads remain, call
crowbar_signal("implementation_complete").`,
			},
			Transitions: []TransitionDef{
				{To: "ai_review", On: "implementation_complete"},
				{To: "spec", On: "scope_changed"},
			},
		},
		{
			Name: "ai_review",
			UI:   UIModeDiff,
			Agent: &AgentDef{
				Intelligence: IntelligenceHigh,
				Tools: []FlowTool{
					ToolCrowbarSignal,
					ToolCrowbarOpenThread,
					ToolCrowbarGetThreads,
					ToolCrowbarResolveThread,
					ToolCrowbarReplyThread,
					ToolFSRead,
				},
				SystemPrompt: `Review the full diff against the spec, repository memory, and coding standards.

First, load all existing threads with crowbar_get_threads() and read their full
history. Do NOT re-raise issues that are already agreed or force-approved.

For each prior thread: verify in the current diff whether the fix was correctly
implemented. If fixed: call crowbar_resolve_thread(thread_id). If incomplete:
reply with crowbar_reply_thread noting what is still missing.

For any new issues not raised in prior rounds: call crowbar_open_thread(file, line, concern).

When your review is complete:
- No open threads: call crowbar_signal("review_passed")
- Open threads remain: call crowbar_signal("review_failed")`,
			},
			Transitions: []TransitionDef{
				{To: "implementation", On: "review_failed"},
				{To: "human_review", On: "review_passed"},
			},
		},
		{
			Name:  "human_review",
			UI:    UIModeDiff,
			Agent: nil, // human-only state
			Transitions: []TransitionDef{
				{To: "implementation", On: "changes_requested"},
				{To: "complete", On: "approved"},
			},
		},
		{
			Name: "debugging",
			UI:   UIModeChat,
			Agent: &AgentDef{
				Intelligence: IntelligenceHigh,
				Tools:        []FlowTool{ToolCrowbarSignal, ToolFSRead, ToolFSWrite, ToolTerminal},
				SystemPrompt: `Help the developer identify and resolve the bug. Call
crowbar_signal("bug_resolved") when fixed. If investigation reveals
broader scope, call crowbar_signal("scope_expanded").`,
			},
			Transitions: []TransitionDef{
				{To: "implementation", On: "bug_resolved"},
				{To: "brainstorming", On: "scope_expanded"},
			},
		},
		{
			Name:     "complete",
			Terminal: true,
		},
	},
}
