package task

import "testing"

func TestTask(t *testing.T) {

	t.Run("フィールド参照で全ての値が取得できることを確認", func(t *testing.T) {
		//todo
	})

	t.Run("done と canceled のとき IsClosed が true になることを確認", func(t *testing.T) {

		tests := map[string]struct {
			Status     Status
			WantResult bool
		}{
			"Statusがdoneの時: ":     {"done", true},
			"Statusがcanceledの時: ": {"canceled", true},
			"Statusがdone/canceledのいずれでもない時（doingを選択）: ": {"doing", false},
		}

		for testName, tt := range tests {
			t.Run(testName, func(t *testing.T) {
				targetTask := Task{Status: tt.Status}
				wantResult := tt.WantResult
				gotResult := targetTask.IsClosed()

				if wantResult != gotResult {
					t.Errorf("Actual Result: %v, Expected Result: %v", gotResult, wantResult)
				}

			})
		}

		//todo
	})

	t.Run("未定義の Status で IsValid が false になることを確認", func(t *testing.T) {
		//todo
	})

}
