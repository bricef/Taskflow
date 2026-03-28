package model

// TagCount represents a tag and how many tasks on a board use it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
