package task

type Status string

const (
	StatusTodo     = "todo"
	StatusDoing    = "doing"
	StatusDone     = "done"
	StatusCanceled = "canceled"
)

func (s *Status) Status() string {
	// todo
	return ""
}

func (s *Status) IsValid() bool {
	// todo'
	return false
}
