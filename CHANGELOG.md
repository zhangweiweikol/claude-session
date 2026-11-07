# 更新日志

本项目的重要变更都记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 新增

- 通过本机 Claude Code CLI 的 print 模式生成会话概要，输出由 JSON Schema 约束，自动探测 `bare` / `lean` / `plain` 三档能力
- 概要采样：保留前 8 条与后 4 条有效用户提问及最后一条回答的结论段，跳过无信息量短句，并对 `sk-`、`ghp_`、`AKIA`、`Bearer`、`password`、`secret` 等敏感串脱敏
- 派生会话增量摘要：只采样分叉点之后的内容；新增不足 3 条消息时直接继承父摘要
- 详情页 AI 概要卡片：未生成 / 生成中 / 失败 / 成功四态，支持重试与重新生成
- 会话列表展示 AI 摘要，搜索命中摘要时高亮，并支持建议标题一键采用
- AI 整理：补齐缺失概要后生成项目级归类建议，可新建或复用多级归类，按置信度预选，支持预览应用与丢弃
- AI 设置：启用、模型、能力探测、最小消息数、字符预算、单轮入队上限、批量大小、结论段、排除项目
- 新增 `/api/ai/*` REST 接口，涵盖概要、状态、探测、设置、任务、归类与建议
- 新增 SQLite 表 `session_ai`、`ai_jobs`、`ai_suggestions`、`ai_folder_suggestions`，并支持旧库迁移
- 设置面板显示 AI 用量与累计成本

### 变更

- 全文索引纳入 AI 摘要文本，扫描新增或重建索引后自动回填
- 会话列表在存在 AI 摘要时优先展示摘要，其次才是最后一条 prompt

## [0.1.0] - 2026-11-06

### 新增

- 扫描 `~/.claude/projects`，索引 JSONL 会话并按 `cwd` 归并为项目
- 会话列表展示标题、最后 prompt、时间、项目路径、消息数、token 用量与 git 分支
- 会话详情：消息时间线、工具调用折叠、导出 JSONL 或 Markdown
- 基于 SQLite FTS5 trigram 的全文搜索
- 按标题、最后 prompt 或会话 ID 的全局搜索
- 会话谱系：解析 `forkedFrom`，展示父子关系与家族树
- 逻辑归类：嵌套文件夹、重命名、移动与排序
- 项目收藏、别名、最近访问与项目搜索
- 重命名会话时追加 `custom-title` 行，兼容 Claude Code
- 通过 CLI 或草稿 JSONL 创建会话
- 分支与副本会话，自动生成名称并记录 `forkedFrom`
- 通过 `claude --resume <id> -p /compact` 压缩上下文
- 回收站：预览、还原与彻底删除
- 在 Windows Terminal + PowerShell 中恢复会话，支持降级
- 通过 Claude Code hooks、SSE 与 30 秒兜底扫描实现实时刷新
- 明暗主题、可折叠面板与可拖拽布局
- 中英文 README、截图与演示 GIF
- 发布打包脚本与按 tag 发布的 GitHub Actions 工作流

[未发布]: https://github.com/zhangweiweikol/claude-session/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/zhangweiweikol/claude-session/releases/tag/v0.1.0