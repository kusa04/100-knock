// package taskはタスクのドメインロジックを管理しています
package task

import "time"

// Taskは管理するタスクの本体のフィールドを管理します
type Task struct {
	ID          string
	Title       string
	Description string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (t *Task) IsClosed() bool {
	//todo
	if t.Status == "done" || t.Status == "canceled" {
		return true
	}
	return false
}
