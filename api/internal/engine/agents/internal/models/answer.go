package models

import (
	"strconv"
	"strings"
	"time"
)

type AnswerCapability struct {
	Wait time.Duration

	Keys []string
}

func (c AnswerCapability) Accepts(key string) bool {
	for _, k := range c.Keys {
		if k == key {
			return true
		}
	}
	return false
}

type AnswerDecision struct {
	Key string

	Answers map[string]any

	Reason string

	Content []byte

	Picked []string
}

const suggestionPrefix = "suggestion-"

func SuggestionOptionID(i int) string { return suggestionPrefix + strconv.Itoa(i) }

func SuggestionIndex(optionID string) (int, bool) {
	n, ok := strings.CutPrefix(optionID, suggestionPrefix)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(n)
	return i, err == nil && i >= 0
}
