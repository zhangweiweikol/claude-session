// Author: zhangweiwei
package trash

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"claude-session/internal/model"
)

// Manager 把会话文件移动到本系统自己的回收站，避免与 Claude Code 的 30 天清理互相影响。
type Manager struct {
	root string
}

func New(root string) *Manager {
	return &Manager{root: root}
}

func (m *Manager) Move(sess model.Session) error {
	dir := filepath.Join(m.root, sess.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建回收站目录失败: %w", err)
	}
	target := filepath.Join(dir, "session.jsonl")
	if err := movePath(sess.FilePath, target); err != nil {
		return fmt.Errorf("移动会话文件失败: %w", err)
	}

	attachments := strings.TrimSuffix(sess.FilePath, ".jsonl")
	if info, err := os.Stat(attachments); err == nil && info.IsDir() {
		if err := movePath(attachments, filepath.Join(dir, "attachments")); err != nil {
			_ = movePath(target, sess.FilePath)
			return fmt.Errorf("移动附件目录失败: %w", err)
		}
	}
	return nil
}

func (m *Manager) Restore(sess model.Session) error {
	dir := filepath.Join(m.root, sess.ID)
	src := filepath.Join(dir, "session.jsonl")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("回收站中找不到会话文件: %w", err)
	}
	if _, err := os.Stat(sess.FilePath); err == nil {
		return errors.New("原位置已存在同名会话文件，请先处理冲突")
	}
	if err := os.MkdirAll(filepath.Dir(sess.FilePath), 0o755); err != nil {
		return err
	}
	if err := movePath(src, sess.FilePath); err != nil {
		return fmt.Errorf("还原会话文件失败: %w", err)
	}

	attachments := strings.TrimSuffix(sess.FilePath, ".jsonl")
	from := filepath.Join(dir, "attachments")
	if _, err := os.Stat(from); err == nil {
		if err := movePath(from, attachments); err != nil {
			return fmt.Errorf("还原附件目录失败: %w", err)
		}
	}
	_ = os.Remove(dir)
	return nil
}

func (m *Manager) Purge(sess model.Session) error {
	return os.RemoveAll(filepath.Join(m.root, sess.ID))
}

func (m *Manager) Path(id string) string {
	return filepath.Join(m.root, id, "session.jsonl")
}

// movePath 优先用 rename；跨盘（Windows 报 not same device）时回退为复制后删除源。
func movePath(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	info, statErr := os.Stat(src)
	if statErr != nil {
		return err
	}
	var copyErr error
	if info.IsDir() {
		copyErr = copyDir(src, dst)
	} else {
		copyErr = copyFile(src, dst)
	}
	if copyErr != nil {
		return copyErr
	}
	return os.RemoveAll(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(from, to); err != nil {
				return err
			}
		} else if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}
