//go:build windows

// Author: zhangweiwei
package ai

import (
	"os/exec"
	"syscall"
)

// hideConsole 让后台调用的 claude 子进程不创建控制台窗口。
// 发布版以 -H windowsgui 编译，自身没有控制台；若不指定 CREATE_NO_WINDOW，
// Windows 会给控制台子进程新开一个终端窗口。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
