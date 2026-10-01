package main

import (
	"fmt"
	"time"
)

func TimeCheck() {
	// monotonic clock("m=...の部分")がある
	time := time.Now()
	fmt.Println(time) // 2026-09-30 21:43:06.741396 +0900 JST m=+0.000135417

	// Round, UTC, Inといったプロセスを跨ぐとmonotonic clcokは消える
	// monotonic clockは同一プロセス内での経過時間計測用のため、プロセスを跨ぐと意味が無いから
	roundTime := time.Round(0)
	fmt.Println(roundTime) // 2026-09-30 21:43:06.741396 +0900 JST

	utcTime := time.UTC()
	fmt.Println(utcTime) // 2026-09-30 12:43:06.741396 +0000 UTC

	inTime := time.In(time.Location())
	fmt.Println(inTime) // 2026-09-30 21:43:06.741396 +0900 JST

}

func main() {
	TimeCheck()

	// timeの初期値を確認
	var initTime time.Time
	fmt.Println(initTime)
}
