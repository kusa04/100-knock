// package taskはタスクのドメインロジックを管理しています
package task

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrEmptyTitle         = errors.New("title is empty")
	ErrTitleTooLong       = errors.New("title is too long")
	ErrDescriptionTooLong = errors.New("description is too long")
)

// Taskは管理するタスクの本体のフィールドを管理します
type Task struct {
	ID          string
	Title       string
	Description string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewTask(title, description string) (*Task, error) {
	// id
	id := "" //todo あとで埋める

	// title
	strippedTitle := strings.TrimSpace(title)
	numOfTitle := utf8.RuneCountInString(strippedTitle)

	// description
	strippedDescription := strings.TrimSpace(description)
	numOfDescription := utf8.RuneCountInString(strippedDescription)

	status := StatusTodo

	var createdAt time.Time // todo あとで埋める
	var updatedAt time.Time // todo あとで埋める

	// titleが空ならerr
	if strippedTitle == "" {
		return nil, ErrEmptyTitle
	}
	// titleが201文字以上ならerr
	if numOfTitle >= 201 {
		return nil, ErrTitleTooLong
	}
	// descriptionが2001以上ならerr
	if numOfDescription >= 2001 {
		return nil, ErrDescriptionTooLong
	}
	return &Task{id, strippedTitle, strippedDescription, status, createdAt, updatedAt}, nil

}

func (t *Task) IsClosed() bool {
	//todo
	if t.Status == "done" || t.Status == "canceled" {
		return true
	}
	return false
}
