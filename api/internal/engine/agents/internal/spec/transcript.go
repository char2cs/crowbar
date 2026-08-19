package spec

// TranscriptSpec declares how to read the provider's OWN record of a
// conversation — claude's session .jsonl, codex's rollout file — which every one
// of their hook payloads names and which Crowbar previously never opened.
//
// It exists because a turn's terminating hook reports ONE message. Claude's Stop
// carries `last_assistant_message`, which is literally the last one: an agent
// that says something, works for thirty seconds and then says something else has
// its first message thrown away, and the record Crowbar serves to sibling agents
// through get_chat_log is a lossy transcript of what the agent actually said.
// The provider's own file is the only source that has the rest.
//
// It is READ-ONLY, and that is the whole reason this is allowed to exist at all.
// The provider home is the provider's; Crowbar opens one file there for reading
// and writes nothing — no lock, no temp file, no truncation, no directory entry.
//
// A provider declaring nothing here behaves EXACTLY as it did before this
// existed: the terminating hook's one message is the whole reply. That is not a
// courtesy, it is the degradation contract — see Declared.
type TranscriptSpec struct {
	// PathField is the hook-payload path carrying the transcript's location. It
	// is read from the payload rather than derived, because a path Crowbar
	// computed itself would be a second, silently divergent copy of the
	// provider's own directory layout.
	PathField string `yaml:"path_field"`

	// Format is the file's encoding. "jsonl" — one JSON object per line — is the
	// only one the engine reads, and is what both shipped providers write. An
	// unrecognised format reads as no transcript at all.
	Format string `yaml:"format"`

	// Message says which entries are the agent's words and where the words are.
	Message TranscriptMessageSpec `yaml:"message"`

	// MaxReadBytes bounds ONE read of the file. It exists because the window
	// between two reads is not bounded by anything the provider promises: a
	// daemon that missed a session's hooks, or a turn that produced a very large
	// tool transcript, must not pull the whole file into memory. Zero takes the
	// engine's own default.
	MaxReadBytes int `yaml:"max_read_bytes"`

	// MaxMessageBytes bounds ONE message's text. A message past it is recorded
	// truncated and MARKED, never silently clipped: a reader must be able to see
	// that there was more. Zero takes the engine's own default.
	MaxMessageBytes int `yaml:"max_message_bytes"`
}

// TranscriptMessageSpec recognises an assistant message inside one transcript
// entry and extracts its text.
//
// Nothing here is interpreted by the engine. Match and Reject are pairs of
// dotted path and expected scalar, compared as text; Blocks names an array and
// BlockMatch/BlockText read inside each element. So the engine learns "an entry
// whose `type` is `assistant`" without ever learning what an assistant is, and a
// provider that reshapes its file is a YAML edit rather than a Go branch.
type TranscriptMessageSpec struct {
	// Match is every path=value pair an entry must satisfy to be a message.
	Match map[string]string `yaml:"match"`

	// Reject is every path=value pair that DISQUALIFIES an entry that otherwise
	// matched. It is separate from Match because the interesting exclusions are
	// fields that are absent on an ordinary entry — claude marks a subagent's
	// messages `isSidechain: true` and an API failure `isApiErrorMessage: true`,
	// and neither key is present at all on a normal one, so no Match pair can
	// express "not that".
	Reject map[string]string `yaml:"reject"`

	// Blocks names the array of content blocks inside one entry. Empty means the
	// text is a plain leaf at Text instead.
	Blocks string `yaml:"blocks"`
	// BlockMatch is Match, applied to one block: it selects the blocks that are
	// words rather than reasoning or a tool call.
	BlockMatch map[string]string `yaml:"block_match"`
	// BlockText is the path to a selected block's text.
	BlockText string `yaml:"block_text"`

	// Text is the path to the whole entry's text, for a provider that does not
	// split a message into blocks. Ignored when Blocks is set.
	Text string `yaml:"text"`

	// Join separates the text of two blocks of the SAME entry, which is one
	// message however many blocks carry it.
	Join string `yaml:"join"`

	// Timestamp is the path to when the message was said. It is used verbatim so
	// a message drained late is dated when the agent said it rather than when
	// Crowbar got round to reading it. Unparseable or absent falls back to the
	// read's own clock, which is never worse than having no time at all.
	Timestamp string `yaml:"timestamp"`
}

// Declared reports whether this provider's transcript can be read at all.
//
// It is the single gate the whole feature hangs off: false means every caller
// behaves as it did before the transcript existed, so "a provider declaring
// nothing is unchanged" is one condition rather than a property spread over
// every call site.
func (t TranscriptSpec) Declared() bool {
	if t.PathField == "" || t.Format != "jsonl" {
		return false
	}
	if len(t.Message.Match) == 0 {
		return false
	}
	return t.Message.Blocks != "" || t.Message.Text != ""
}
