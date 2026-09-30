package task

// Statusはtaskの状態（status）を管理しています
type Status string

const (
	StatusTodo     Status = "todo"
	StatusDoing    Status = "doing"
	StatusDone     Status = "done"
	StatusCanceled Status = "canceled"
)

func (s Status) String() string {
	return string(s)
}

func (s Status) IsValid() bool {
	switch s {
	case StatusTodo, StatusDoing, StatusDone, StatusCanceled:
		return true
	default:
		return false
	}
}
