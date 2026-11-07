<#
.SYNOPSIS
    GitHub 开源项目冷启动体检。

.DESCRIPTION
    检查仓库文件齐全度、项目年龄、版本标签、README 首屏素材和 CI 配置，
    输出缺口清单，对应 Obsidian 笔记《GitHub 开源项目冷启动清单》阶段 0。

.PARAMETER RepoPath
    仓库路径，默认为当前目录。

.EXAMPLE
    .\repo-audit.ps1

.EXAMPLE
    .\repo-audit.ps1 -RepoPath "D:\workspace\go\claude-session"
#>
param(
    [string]$RepoPath = (Get-Location).Path
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $RepoPath)) {
    throw "路径不存在：$RepoPath"
}
$RepoPath = (Resolve-Path -LiteralPath $RepoPath).Path

function Test-RelativeFile([string]$RelativePath) {
    return Test-Path -LiteralPath (Join-Path $RepoPath $RelativePath)
}

function Write-Status([string]$OkText, [string]$MissingText, [bool]$Ok) {
    if ($Ok) {
        Write-Host ("  [OK]   " + $OkText) -ForegroundColor Green
    } else {
        Write-Host ("  [--]   " + $MissingText) -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "=== GitHub 冷启动体检 ===" -ForegroundColor Cyan
Write-Host "仓库：$RepoPath"
Write-Host ""

# ---------- 技术栈 ----------
$stack = "未知"
if (Test-RelativeFile "go.mod") { $stack = "Go" }
elseif (Test-RelativeFile "package.json") { $stack = "Node.js" }
elseif (Test-RelativeFile "pom.xml") { $stack = "Java (Maven)" }
elseif (Test-RelativeFile "build.gradle") { $stack = "Java (Gradle)" }
elseif (Test-RelativeFile "Cargo.toml") { $stack = "Rust" }
elseif (Test-RelativeFile "pyproject.toml") { $stack = "Python" }
Write-Host "技术栈：$stack"
Write-Host ""

# ---------- 文件齐全度 ----------
Write-Host "--- 文件 ---"
$files = @(
    @{ Path = "README.md";        Level = "P0"; Desc = "项目说明" },
    @{ Path = "LICENSE";          Level = "P0"; Desc = "许可证" },
    @{ Path = ".gitignore";       Level = "P0"; Desc = "忽略规则" },
    @{ Path = "README.en.md";     Level = "P1"; Desc = "英文 README" },
    @{ Path = "CONTRIBUTING.md";  Level = "P1"; Desc = "贡献指南" },
    @{ Path = "SECURITY.md";      Level = "P1"; Desc = "安全策略" },
    @{ Path = "CHANGELOG.md";     Level = "P1"; Desc = "变更日志" }
)
$missingP0 = 0
$missingP1 = 0
foreach ($f in $files) {
    $ok = Test-RelativeFile $f.Path
    Write-Status ("{0,-18} ({1}) {2}" -f $f.Path, $f.Level, $f.Desc) `
                ("{0,-18} ({1}) 缺失 - {2}" -f $f.Path, $f.Level, $f.Desc) $ok
    if (-not $ok) {
        if ($f.Level -eq "P0") { $missingP0++ } else { $missingP1++ }
    }
}

$hasIssueTemplate = (Test-Path (Join-Path $RepoPath ".github/ISSUE_TEMPLATE"))
Write-Status "Issue 模板" "Issue 模板缺失" $hasIssueTemplate

$hasPrTemplate = (Test-RelativeFile ".github/pull_request_template.md") -or (Test-RelativeFile ".github/PULL_REQUEST_TEMPLATE.md")
Write-Status "PR 模板" "PR 模板缺失" $hasPrTemplate

$workflows = @()
$wfDir = Join-Path $RepoPath ".github/workflows"
if (Test-Path $wfDir) {
    $workflows = @(Get-ChildItem -Path (Join-Path $wfDir "*") -File |
        Where-Object { $_.Extension -in ".yml", ".yaml" })
}
Write-Status ("GitHub Actions（{0} 个）" -f $workflows.Count) "GitHub Actions 缺失" ($workflows.Count -gt 0)

$docsDir = Test-Path (Join-Path $RepoPath "docs")
Write-Status "docs/ 文档目录" "docs/ 文档目录缺失" $docsDir
Write-Host ""

# ---------- README 首屏素材 ----------
Write-Host "--- README 首屏 ---"
$readmePath = Join-Path $RepoPath "README.md"
if (Test-Path $readmePath) {
    $readme = Get-Content -LiteralPath $readmePath -Raw -Encoding UTF8
    $hasImage = $readme -match '!\[[^\]]*\]\([^)]+\.(png|jpe?g|gif|webp|svg)'
    $hasBadge = $readme -match 'img\.shields\.io|badge'
    $hasQuickStart = $readme -match '(?im)^##+\s*(quick\s*start|快速开始|getting\s*started|运行)'
    $hasCodeBlock = $readme -match '```'

    Write-Status "README 含截图/GIF" "README 无截图/GIF（首屏转化会明显下降）" $hasImage
    Write-Status "README 含徽章" "README 无徽章（可选）" $hasBadge
    Write-Status "README 含快速开始" "README 无快速开始章节" $hasQuickStart
    Write-Status "README 含代码块" "README 无代码块" $hasCodeBlock
} else {
    Write-Host "  [!!]   README.md 不存在，无法检查首屏" -ForegroundColor Red
}
Write-Host ""

# ---------- Git 状态 ----------
Write-Host "--- Git ---"
$firstDate = & git -C $RepoPath log --reverse --format=%ad --date=short 2>$null | Select-Object -First 1
if ($firstDate) {
    $days = ((Get-Date).Date - ([datetime]$firstDate).Date).Days
    Write-Host ("首次提交：{0}（{1} 天）" -f $firstDate, $days)
    if ($days -ge 14) {
        Write-Host "  [OK]   已满 14 天，可投 awesome list" -ForegroundColor Green
    } else {
        Write-Host ("  [--]   未满 14 天，awesome list 需等到 {0}" -f ([datetime]$firstDate).AddDays(14).ToString("yyyy-MM-dd")) -ForegroundColor Yellow
    }
} else {
    Write-Host "  [!!]   无法读取 git 历史" -ForegroundColor Red
}

$tags = @(& git -C $RepoPath tag --sort=-creatordate 2>$null)
if ($tags.Count -gt 0) {
    Write-Host ("标签：" + ($tags[0..([Math]::Min(4, $tags.Count - 1))] -join ", "))
} else {
    Write-Host "  [--]   尚无版本标签" -ForegroundColor Yellow
}

$remotes = @(& git -C $RepoPath remote 2>$null)
if ($remotes.Count -gt 0) {
    Write-Host ("远程：" + ($remotes -join ", "))
} else {
    Write-Host "  [--]   未配置远程" -ForegroundColor Yellow
}

$dirty = & git -C $RepoPath status --porcelain 2>$null
if ($dirty) {
    Write-Host "  [--]   工作区有未提交改动" -ForegroundColor Yellow
}
Write-Host ""

# ---------- 建议 ----------
Write-Host "--- 建议 ---" -ForegroundColor Cyan
$suggestions = @()
if ($missingP0 -gt 0) { $suggestions += "P0：补齐必需文件（README / LICENSE / .gitignore）" }
if ($missingP0 -eq 0) { $suggestions += "P0：门面基本齐全，检查首屏截图与快速开始命令" }
if ($missingP1 -gt 0) { $suggestions += "P1：补齐 CONTRIBUTING / SECURITY / CHANGELOG / 英文 README" }
if ($workflows.Count -eq 0) { $suggestions += "P1：添加 GitHub Actions（构建 + 测试 + tag 自动发布）" }
if ($tags.Count -eq 0) { $suggestions += "P1：打首个版本标签并创建 Release，上传构建产物与 SHA256" }
if ($tags.Count -gt 0) { $suggestions += "P1：确认 Release 资产可下载、SHA256 可校验" }
$suggestions += "P2：仓库元数据（About / Topics / Social preview）"
if ($days -ne $null -and $days -lt 14) {
    $suggestions += ("P3：awesome list 暂不可投，{0} 后再提交" -f ([datetime]$firstDate).AddDays(14).ToString("yyyy-MM-dd"))
} else {
    $suggestions += "P3：满足门槛后提交 1-2 个 awesome list"
}
$suggestions += "P3：准备 2-3 篇技术文章，首发掘金 / dev.to"

foreach ($s in $suggestions) {
    Write-Host ("  - " + $s)
}
Write-Host ""
