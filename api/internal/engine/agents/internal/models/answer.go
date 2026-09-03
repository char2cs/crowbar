package models

import "time"

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
}
