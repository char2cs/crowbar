package git

import "time"

type Stash struct {
	ID           string    `json:"id"`
	Message      string    `json:"message"`
	Date         time.Time `json:"date"`
	FilesChanged int       `json:"filesChanged"`
}
