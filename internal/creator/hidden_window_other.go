//go:build !windows

// Author: zhangweiwei
package creator

import "os/exec"

// hideConsole 在非 Windows 平台无需处理控制台窗口。
func hideConsole(cmd *exec.Cmd) {}
