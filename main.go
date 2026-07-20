// macinv — macOS 설치 소프트웨어 인벤토리 CLI.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kyungryeol-yoon/macinv/internal/cmd"
)

func main() {
	if err := cmd.NewRootCmd().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}
