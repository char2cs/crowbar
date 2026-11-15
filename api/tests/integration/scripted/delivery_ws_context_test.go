//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// leadDescriptor is wsDescriptor for a provider whose only channel for Crowbar's context is
// the user text of a session's first prompt, and whose lost-session error is code 4001.
func leadDescriptor(endpoint string) []byte {
	doc := fmt.Sprintf(wsDescriptor, endpoint)
	doc = strings.Replace(doc, "rpc_code: -32600", "rpc_code: 4001", 1)
	doc = strings.Replace(doc, `, context: "{context}"`, "", 1)
	return []byte(doc + "inject:\n  - at: context\n    prompt_lead: \"{context}\"\n")
}

func newLeadRig(t *testing.T) *socketRig {
	t.Helper()
	return newSocketRigFor(t, wsProvider, leadDescriptor)
}

// sentText is the text of the turn/send the daemon wrote next.
func (s *socketRig) sentText(t *testing.T) string {
	t.Helper()
	var params struct {
		Text string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(s.nextMethod(t, "turn/send").Params, &params))
	return params.Text
}

// answerTurn plays the provider's side of a turn on session: it echoes the prompt it was
// sent and finishes with reply.
func (s *socketRig) answerTurn(t *testing.T, chatID, session, sent, reply string) {
	t.Helper()
	s.push(t, fmt.Sprintf(`{"method":"item/user","params":{"session_id":%q,"text":%q}}`, session, sent))
	s.push(t, fmt.Sprintf(`{"method":"turn/done","params":{"session_id":%q,"text":%q}}`, session, reply))
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: "+reply) && !s.working(chatID) },
		"the turn never closed")
}

// userSaid is every user message of chatID's ledger, as typed.
func (s *socketRig) userSaid(chatID string) []string {
	var out []string
	for _, line := range s.said(chatID) {
		if text, ok := strings.CutPrefix(line, "user: "); ok {
			out = append(out, text)
		}
	}
	return out
}

// A session the provider no longer has is replaced by a new one, and a provider that takes
// no instructions at all is handed Crowbar's record of the conversation as the leading
// section of that session's first prompt: once, never as something the person typed.
func TestRegression_LostSessionReplacementCarriesTheRecordOnItsFirstPrompt(t *testing.T) {
	s := newLeadRig(t)
	chatID := s.spawn(wsProvider)

	require.NoError(t, s.send(chatID, "remember the word PELICAN"))
	first := s.sentText(t)
	assert.True(t, strings.HasSuffix(first, "\n\nremember the word PELICAN"), "a new session's first prompt carries its context first")
	s.answerTurn(t, chatID, "sess-1", first, "noted")

	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)
	s.reject.Store("session/resume", "no such session")
	err := s.send(chatID, "what was the word?")
	require.Error(t, err)
	assert.Equal(t, problem.SessionLost, problem.CodeOf(err))

	s.reject.Delete("session/resume")
	s.results.Store("session/new", `{"id":"sess-2"}`)
	s.offerEndpoint(t)
	require.NoError(t, s.send(chatID, "start again"))
	replacement := s.sentText(t)
	assert.True(t, strings.HasSuffix(replacement, "\n\nstart again"), "the typed text ends the prompt: %q", replacement)
	assert.Contains(t, replacement, "PELICAN", "the replacement is handed the record")
	assert.Contains(t, replacement, "noted")
	s.answerTurn(t, chatID, "sess-2", replacement, "PELICAN")

	require.NoError(t, s.send(chatID, "and again"))
	assert.Equal(t, "and again", s.sentText(t), "the record rides the session's first prompt only")
	s.answerTurn(t, chatID, "sess-2", "and again", "done")

	assert.Equal(t, []string{ledger.MarkerFromRecord}, s.markers(chatID))
	assert.Equal(t, []string{"remember the word PELICAN", "start again", "and again"}, s.userSaid(chatID),
		"the ledger holds what was typed, never the record that rode with it")
}

// A resumed session has its own history: reviving a chat that lost nothing hands it no record.
func TestRegression_CleanResumeCarriesNoRecordOnItsPrompt(t *testing.T) {
	s := newLeadRig(t)
	chatID := s.spawn(wsProvider)
	require.NoError(t, s.send(chatID, "first"))
	s.answerTurn(t, chatID, "sess-1", s.sentText(t), "one")
	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)

	require.NoError(t, s.send(chatID, "second"))

	s.nextMethod(t, "session/resume")
	assert.Equal(t, "second", s.sentText(t))
}
