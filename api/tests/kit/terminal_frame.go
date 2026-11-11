package kit

import "github.com/char2cs/crowbar/api/internal/core/terminal"

// ParseTerminalFrame splits one binary terminal output message into its payload and
// whether it is a snapshot; ok is false for non-output frames. The daemon only writes
// these frames, so the decoder lives with the test clients.
func ParseTerminalFrame(msg []byte) (payload []byte, snapshot, ok bool) {
	if len(msg) == 0 || msg[0] > terminal.FrameSnapshot {
		return nil, false, false
	}
	return msg[1:], msg[0] == terminal.FrameSnapshot, true
}
