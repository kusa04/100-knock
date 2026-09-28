// Package main は API サーバーのエントリポイントです。
// 問題を進めるにつれて HTTP サーバー、gRPC サーバー、DI(Wire)へと育てていきます。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "go-backend-100-knocks: 環境は整っています。problems/01-beginner.md の第1問から始めてください。")
}
