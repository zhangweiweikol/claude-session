# 更新日志

本项目的重要变更都记录在此文件中。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 变更

- AI 整理提速：概要阶段改为 4 路并行、默认批量 15 条；归类阶段 3 路并发，单批独立超时、总预算按批数动态计算，单批失败不再影响其它批次
- 手动整理单轮上限从 50 提高到 200 条
- AI 整理按钮显示「生成概要 / 整理归类 x/y」实时进度，失败或部分失败时给出提示

### 新增

- AI 归类按谱系统一：同一 fork/branch 谱系（含多级派生）的会话始终归入同一个归类；父会话已归类（含手动归类）时子会话跟随，手动归类不被改写
- 每个项目记住上次选择的归类视图，切换项目时沿用；没有记录时默认回到「已归类」

### 修复

- 新建会话时「归类」下拉框默认跟随左侧归类列表当前选中的归类，不再总是回到「待归类」
- 修复 AI 整理按谱系切批导致无谱系会话各自单独成批、无法归类的问题；多个谱系组现在共享同一批次，不足 2 个会话的新归类在预览中明确显示为保持未归类
- Windows 下同一目录因 `cwd` 大小写不同被拆成多个项目：扫描时把路径还原为磁盘真实大小写，启动时自动合并已存在的重复项目（会话、归类、别名、收藏等元数据一并迁移）

## [0.2.0] - 2026-11-07

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

[未发布]: https://github.com/zhangweiweikol/claude-session/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/zhangweiweikol/claude-session/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/zhangweiweikol/claude-session/releases/tag/v0.1.0