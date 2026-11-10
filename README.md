# Claude Code 会话管理

[English](README.en.md) | 简体中文

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)

以项目为单位浏览、归类、恢复本机 Claude Code 会话的本地 Web 工具，并可用本机 Claude Code 生成会话概要与归类建议。

Go + SQLite + 原生前端，单进程、单二进制，只监听 `127.0.0.1`。

![界面截图](docs/screenshot.png)

![操作演示](docs/demo.gif)

> 本项目是为了学习 Go 开发而做的练习项目，欢迎指正并一起学习。

## 特性

- **项目归并**：扫描 `~/.claude/projects`，按 jsonl 中的 `cwd` 归并项目，列表展示标题、最后 prompt、时间、消息数、token 用量、git 分支；Windows 下仅大小写不同的路径会还原为磁盘真实大小写并合并为同一个项目
- **全文搜索**：FTS5 trigram（支持中文子串），也可按标题 / prompt / 会话 ID 搜索，`Ctrl+K` 唤起
- **逻辑归类**：多层嵌套分组、移动、重命名，只写 SQLite，不改动 Claude Code 的目录结构；切换项目时默认回到「已归类」，并记住每个项目上次选择的归类视图
- **谱系视图**：解析 `forkedFrom`，展示派生关系、家族树、分叉点与完整血缘链
- **会话操作**：恢复、重命名、压缩、分支、副本、批量删除与回收站还原
- **AI 概要**：调用本机 Claude Code 生成会话摘要与建议标题，列表与详情页直接展示
- **AI 整理**：生成项目级归类建议，预览勾选后应用，手动归类始终不受影响
- **实时刷新**：Claude Code hooks 触发增量扫描，SSE 推送更新，30 秒定时扫描兜底
- **本地优先**：jsonl 是唯一真值，删除数据库不影响 Claude Code；只监听本机并校验 Origin

## 环境要求

- Windows（终端集成面向 Windows Terminal + PowerShell）
- Go 1.27+（源码构建）
- Claude Code 已安装并登录，会话位于 `~/.claude/projects`

## 快速开始

### 发布包

- 双击 `session-deck.exe` 或 `start.bat`，程序在后台运行并自动打开 <http://127.0.0.1:7788>
- 日志写入 `data/session-deck.log`
- 停止：双击 `stop.bat`，或在任务管理器结束 `session-deck.exe`

### 从源码运行

```bash
go run ./cmd/server
```

### 构建

```bash
# 发布版：GUI 子系统，无控制台窗口
go build -trimpath -ldflags "-H windowsgui -s -w" -o bin/session-deck.exe ./cmd/server

# 调试版：保留控制台输出
go build -o bin/session-deck-debug.exe ./cmd/server
```

## 配置

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-addr` | `127.0.0.1:7788` | 监听地址 |
| `-db` | `data/sessions.db` | SQLite 路径 |
| `-projects` | `~/.claude/projects` | Claude Code 会话目录 |

## 使用说明

### 快捷键

| 快捷键 | 功能 |
|---|---|
| `Ctrl+K` | 聚焦搜索 |
| `Ctrl+P` | 搜索项目 |

### AI 概要与整理

- **概要**：详情页展示目标、关键动作、结论与话题标签；未生成 / 生成中 / 失败 / 成功四态齐全，可重试或重新生成；列表优先展示 AI 摘要，搜索命中时高亮
- **建议标题**：会话没有自定义标题时给出建议，详情页和列表均可一键采用
- **AI 整理**：按项目补齐缺失概要 → 生成归类建议 → 预览勾选应用；可新建多级归类或复用已有归类，置信度低于 0.6 默认不勾选，也可丢弃本次结果
- **派生会话**：分叉后新增不足 3 条消息时继承父摘要，否则只采样分叉点之后的内容
- **采样与隐私**：前 8 + 后 4 条有效提问 + 结论段，默认 1500 字符预算；跳过「继续」「好的」等短句，对 `sk-` / `ghp_` / `AKIA` / `Bearer` / `password` 等敏感串脱敏
- **调用与成本**：`claude -p --output-format json --no-session-persistence --tools ""`，JSON Schema 约束输出，自动探测 `bare` / `lean` / `plain` 档位；批量默认 10 条，失败拆单重试，设置页显示用量与累计成本
- **设置**：启用、模型、档位探测、最小消息数、字符预算、单轮入队上限、批量大小、结论段、排除项目

### 打开终端

`POST /api/sessions/{id}/open` 复用最近的 Windows Terminal 窗口：

```
wt.exe -w 0 nt -d <项目路径> pwsh.exe -NoExit -EncodedCommand <PowerShell Base64>
```

降级顺序：`pwsh` + Terminal → `powershell` + Terminal → `cmd /c start` 新窗口 → 返回错误并保留可复制的恢复命令。

安全约束：会话 ID 必须是 UUID；项目路径必须来自已索引项目，且不含引号、换行、NUL；命令以参数数组传递，不拼接 shell 字符串。

## API

```
GET    /api/health
GET    /api/projects
POST   /api/projects                    { path }
PATCH  /api/projects/{id}               { pinned?, alias? }
POST   /api/projects/{id}/move          { direction: up|down }
POST   /api/projects/{id}/touch
DELETE /api/projects/{id}
GET    /api/projects/{id}/folders
POST   /api/projects/{id}/folders      { name, parent_id? }
PATCH  /api/projects/{id}/folders/{folder}   { name }
POST   /api/projects/{id}/folders/{folder}/move  { direction: up|down }
DELETE /api/projects/{id}/folders/{folder}
GET    /api/sessions?project_id=&folder_id=&q=&deleted=&limit=
POST   /api/sessions                   { project_id, folder_id, title, first_message, mode }
POST   /api/sessions/batch-delete      { ids, permanent }
GET    /api/sessions/{id}
GET    /api/sessions/{id}/ancestors
GET    /api/sessions/{id}/export       ?format=json|md
PATCH  /api/sessions/{id}              { folder_id?, title? }
DELETE /api/sessions/{id}              ?permanent=1
POST   /api/sessions/{id}/open
POST   /api/sessions/{id}/branch
POST   /api/sessions/{id}/fork
POST   /api/sessions/{id}/compact
POST   /api/projects/{id}/trash/purge
POST   /api/sessions/{id}/restore
POST   /api/ai/summarize               { project_id?, ids?, force? }
GET    /api/ai/status                  ?project_id=
POST   /api/ai/probe
GET    /api/ai/settings
PATCH  /api/ai/settings                { enabled?, model?, min_messages?, max_chars?, … }
GET    /api/ai/jobs/{id}
POST   /api/ai/categorize              { project_id?, reevaluate? }
GET    /api/ai/suggestions             ?job_id=
POST   /api/ai/suggestions/apply       { job_id, folder_ids?, session_ids? }
POST   /api/ai/suggestions/discard     { job_id }
GET    /api/fs/dirs?path=
POST   /api/sync
POST   /api/hooks/refresh
GET    /api/hooks/config
GET    /api/events                     SSE
```

- `mode` 取 `cli`（调用 `claude --session-id`）或 `draft`（只写 jsonl）
- `deleted=1` 查询回收站；`folder_id=unfiled` 表示待归类
- AI 接口：`summarize` 不传 `project_id` 时使用最近 10 分钟内打开的项目，`force=1` 重新生成；`categorize` 的 `reevaluate=true` 重新评估已有 AI 归类；`apply` 不传 ID 时应用全部建议

## 数据原则

- **jsonl 是唯一真值**：SQLite 只保存索引、归类、AI 摘要等本地元数据，删除数据库不影响 Claude Code
- **归类是逻辑的**：文件夹只存在 SQLite 中，不改变 `~/.claude/projects` 的目录结构
- **派生关系只读**：`parent_id` / `fork_point_uuid` 从 jsonl 首行的 `forkedFrom` 解析，不修改会话文件；父会话被删除后关系保留，界面显示为缺失
- **AI 数据只存本地**：概要、任务与建议存在 `session_ai` / `ai_jobs` / `ai_suggestions` / `ai_folder_suggestions`，不修改 jsonl；扫描后会把摘要回填进全文索引
- **写入保护**：重命名、删除前检查 `~/.claude/sessions/*.json`，会话运行中返回 409；删除先进 `~/.claude-session/trash/<session-id>/`，可还原
- **30 天清理**：Claude Code 清理过期 transcript 后，找不到文件的会话标记为 `missing`，本地归类记录保留

## 目录结构

```
cmd/server/          启动入口
internal/api/        HTTP 路由、SSE、hooks
internal/ai/         AI 概要、归类建议、CLI 调用与任务队列
internal/creator/    CLI / 草稿方式创建会话
internal/scanner/    jsonl 扫描、元数据解析、概要采样、标题写入
internal/store/      SQLite 索引、归类、全文检索、AI 摘要
internal/trash/      回收站文件移动与还原
internal/opener/     终端与 PowerShell 启动
internal/model/      数据模型
web/                 前端（embed 进二进制）
data/                SQLite 数据库（运行时生成）
```

## 实时刷新

服务提供 `POST /api/hooks/refresh`，配置片段可从 `GET /api/hooks/config` 获取：

```json
{
  "hooks": {
    "Stop": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ],
    "SessionStart": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ],
    "SessionEnd": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ]
  }
}
```

合并进 `~/.claude/settings.json` 后，每次回复结束、会话开始或结束都会触发增量扫描，页面通过 SSE 自动刷新。未配置 hooks 时仍有 30 秒定时扫描兜底。

## 路线图

- 会话内容编辑
- 标签与批量操作
- 多实例 / 远程访问

## 许可

[Apache-2.0](LICENSE)