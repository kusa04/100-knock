package task

import "time"

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
