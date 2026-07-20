// Package cmd는 cobra 기반 CLI 커맨드를 정의한다(출력 레이어).
// 스캔 로직은 internal/source, 데이터 모델/요약은 internal/report에 있다.
package cmd

import (
	"github.com/spf13/cobra"
)

// Version은 macinv 버전이다(`macinv --version`).
const Version = "0.1.0"

// NewRootCmd는 macinv 루트 커맨드를 만든다.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "macinv",
		Short:         "macOS 설치 소프트웨어 인벤토리 CLI",
		Long:          "내 맥에 뭐가 어디 깔려있는지 스캔하고, 업데이트 대기를 표시하고, 안전하게 정리한다.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newScanCmd())
	root.AddCommand(newOutdatedCmd())
	root.AddCommand(newTraceCmd())
	root.AddCommand(newRuntimesCmd())
	root.AddCommand(newCleanCmd())
	root.AddCommand(newOrphansCmd())
	root.AddCommand(newUpdateCmd())
	root.AddCommand(newTuiCmd())
	return root
}
