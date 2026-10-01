package task

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTask(t *testing.T) {

	t.Run("NewTaskのバリデーション正常に通ることを確認", func(t *testing.T) {
		tests := map[string]struct {
			title      string
			descrption string
			wantErr    error
		}{
			// title
			"titleが空の時はErrEmptyTitleを返すことを確認":             {"", "オンライン参加", ErrEmptyTitle},
			"titleがの前後が空白の時はErrEmptyTitleを返すことを確認":        {"  ", "オンライン参加", ErrEmptyTitle},
			"titleが1文字の時はErrEmptyTitleが返らないことを確認":         {"a", "オンライン参加", nil},
			"titleが201文字以上の時はErrTitleTooLongを返すことを確認":     {strings.Repeat("a", 201), "オンライン参加", ErrTitleTooLong},
			"titleが200文字以下の時はErrTitleTooLongが返らないことを確認":   {strings.Repeat("a", 200), "オンライン参加", nil},
			"titleが全角200文字以下の時はErrTitleTooLongが返らないことを確認": {strings.Repeat("あ", 200), "オンライン参加", nil},

			// description
			"descriptionが2001以上の時はErrDescriptionTooLongを返すことを確認":   {"1on1ミーティング", strings.Repeat("a", 2001), ErrDescriptionTooLong},
			"descriptionが2000以下の時はErrDescriptionTooLongを返さないことを確認": {"1on1ミーティング", strings.Repeat("b", 2000), nil},
		}

		for testName, tt := range tests {
			t.Run(testName, func(t *testing.T) {
				_, gotErr := NewTask(tt.title, tt.descrption)

				if !errors.Is(gotErr, tt.wantErr) {
					t.Errorf("Got Err: %v, Want Err: %v", gotErr, tt.wantErr)
				}
			})
		}
	})

	t.Run("フィールド参照で全ての値が取得できることを確認", func(t *testing.T) {
		id := "1"
		title := "テストMTG"
		description := "kusaとの1on1"
		status := Status(StatusTodo)
		createdAt := time.Now()
		updatedAt := time.Now()

		test := Task{ID: id, Title: title, Description: description, Status: status, CreatedAt: createdAt, UpdatedAt: updatedAt}

		gotID := test.ID
		gotTitle := test.Title
		gotDescription := test.Description
		gotStatus := test.Status
		gotCreatedAt := test.CreatedAt
		gotUpdatedAt := test.UpdatedAt

		if gotID != id {
			t.Errorf("ID: got %v, want %v", gotID, id)
		}
		if gotTitle != title {
			t.Errorf("Title: got %v, want %v", gotTitle, title)
		}
		if gotDescription != description {
			t.Errorf("Description: got %v, want %v", gotDescription, description)
		}
		if gotStatus != status {
			t.Errorf("Status: got %v, want %v", gotStatus, status)
		}
		if !gotCreatedAt.Equal(createdAt) {
			t.Errorf("CreatedAt: got %v, want %v", gotCreatedAt, createdAt)
		}
		if !gotUpdatedAt.Equal(updatedAt) {
			t.Errorf("UpdatedAt: got %v, want %v", gotUpdatedAt, updatedAt)
		}
	})

	t.Run("done と canceled のとき IsClosed が true になることを確認", func(t *testing.T) {

		tests := map[string]struct {
			Status     Status
			WantResult bool
		}{
			"Statusがdoneの時":     {"done", true},
			"Statusがcanceledの時": {"canceled", true},
			"Statusがdone/canceledのいずれでもない時（doingを選択）": {"doing", false},
		}

		for testName, tt := range tests {
			t.Run(testName, func(t *testing.T) {
				targetTask := Task{Status: tt.Status}
				gotResult := targetTask.IsClosed()
				wantResult := tt.WantResult

				if wantResult != gotResult {
					t.Errorf("Got result: %v, Want result: %v", gotResult, wantResult)
				}

			})
		}
	})

	t.Run("未定義の Status で IsValid が false になることを確認", func(t *testing.T) {
		targetTask := Task{Status: "unknown"}
		targetInvalidStatus := targetTask.Status
		if targetInvalidStatus.IsValid() {
			t.Errorf("%v should be invalid", targetInvalidStatus)
		}
	})

}
