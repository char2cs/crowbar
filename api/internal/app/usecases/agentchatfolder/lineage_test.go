package agentchatfolder_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedFolder appends a folder row hanging off parentID.
func seedFolder(
	folders *mocks.AgentChatFolderStore,
	id string,
	parentID string,
) {
	folders.Rows = append(folders.Rows, domain.AgentChatFolder{
		ID: id, WorkspaceID: workspaceID, ParentID: parentID, Name: id,
	})
}

// A drop onto another chat is the moment a chat BECOMES a thread, and it is the
// only moment anything can record when it started reading that chat. The note
// carries the lineage rather than a bare "you moved", because the record is what
// a reader believes afterwards.
func TestPlaceChat_RecordsTheNewLineageInTheChatsOwnConversation(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

// The chain, nearest first — a chat dropped under a thread inherits that
// thread's ancestors too, and the record has to name all of them.
func TestPlaceChat_RecordsTheWholeChainNearestFirst(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedChat(chats, "c3", 3)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c3",
		agentchatfolder.PlaceInput{ParentID: name("c2")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c3", Ancestors: []string{"c2", "c1"}}}, chats.Noted)
}

// Dropping onto a FOLDER inside a chat makes the chat a thread just the same:
// folders are transparent, so the record names the chat above the folder and
// never the folder.
func TestPlaceChat_AFolderInsideAChatStillRecordsTheChat(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedFolder(folders, "outer", "c1")
	seedFolder(folders, "inner", "outer")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("inner")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

// GAINED, not merely changed. Filing a thread into a folder under the SAME
// parent leaves it reading exactly what it read a moment ago, and announcing a
// context change there would be announcing one that did not happen.
func TestPlaceChat_FilingAThreadUnderItsOwnParentRecordsNothing(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedFolder(folders, "notes", "c1")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("notes")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted, "the chat reads the same chat it read before; nothing happened to record")
}

// A chat dragged OUT from under its parent gains nothing. What it no longer
// reads is answered by its next spawn resolving an empty lineage — there is no
// new context to date the start of.
func TestPlaceChat_MovingAThreadBackToTheRootRecordsNothing(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

// A pure reorder is not a lineage change either.
func TestPlaceChat_AReorderRecordsNothing(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		agentchatfolder.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

// The rows have already moved by the time the note is written, so a note that
// fails must not report the move as failed. The relationship rides on ParentID;
// what is lost is the line in the record, not the behaviour it describes.
func TestPlaceChat_AFailedNoteDoesNotFailTheMove(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.NoteErr = errors.New("ledger unwritable")

	placed, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, "c1", chatRow(t, chats, "c2").ParentID, "and the move stands")
}
