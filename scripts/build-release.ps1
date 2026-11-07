<#
.SYNOPSIS
    构建 Windows 发布产物并生成 SHA256 校验文件。

.DESCRIPTION
    编译 Go 项目，把可执行文件、README、LICENSE 以及可选的 start.bat / stop.bat
    一起打包为 zip，并生成 SHA256SUMS。输出可直接作为 GitHub Release 资产。

    若 main 包中定义了 `var version string`，会自动注入版本号。

.PARAMETER Version
    版本号，建议与 git tag 一致，例如 v0.1.0。

.PARAMETER MainPackage
    main 包路径，默认为 ./cmd/server。

.PARAMETER AppName
    产物名称，默认取仓库目录名（小写）。

.PARAMETER OutputDir
    输出目录，默认为 dist。

.PARAMETER Console
    保留控制台窗口（调试构建），不添加 -H windowsgui。

.EXAMPLE
    .\scripts\build-release.ps1 -Version v0.1.0

.EXAMPLE
    .\scripts\build-release.ps1 -Version v0.2.0 -MainPackage ./cmd/server -Console
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [string]$MainPackage = "./cmd/server",
    [string]$AppName = "",
    [string]$OutputDir = "dist",
    [switch]$Console
)

$ErrorActionPreference = "Stop"

if ($Version -notmatch '^v?\d+\.\d+\.\d+([-+].*)?$') {
    throw "版本号格式不正确：$Version（示例：v0.1.0）"
}
if (-not $Version.StartsWith("v")) { $Version = "v" + $Version }

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ($AppName -eq "") {
    $AppName = (Split-Path $RepoRoot -Leaf).ToLower()
}

Push-Location $RepoRoot
try {
    if (-not (Test-Path (Join-Path $RepoRoot "go.mod"))) {
        throw "当前目录不是 Go 模块（找不到 go.mod）：$RepoRoot"
    }

    $OutputPath = Join-Path $RepoRoot $OutputDir
    $StageName = "release-$Version"
    $StagePath = Join-Path $OutputPath $StageName
    $ZipName = "$AppName-$Version-windows-amd64.zip"
    $ZipPath = Join-Path $OutputPath $ZipName

    Write-Host "仓库：$RepoRoot"
    Write-Host "版本：$Version"
    Write-Host "产物：$ZipName"

    if (Test-Path $StagePath) { Remove-Item $StagePath -Recurse -Force }
    New-Item -ItemType Directory -Path $StagePath -Force | Out-Null
    New-Item -ItemType Directory -Path $OutputPath -Force | Out-Null

    # 版本注入：仅当 main 包定义了 var version 时生效
    $mainDir = Join-Path $RepoRoot $MainPackage
    $hasVersionVar = $false
    if (Test-Path $mainDir) {
        $goFiles = Get-ChildItem -Path $mainDir -Filter *.go -File
        foreach ($f in $goFiles) {
            $content = Get-Content -LiteralPath $f.FullName -Raw -Encoding UTF8
            if ($content -match 'var\s+version\s+string') { $hasVersionVar = $true; break }
        }
    }

    $ldflags = if ($Console) { "-s -w" } else { "-H windowsgui -s -w" }
    if ($hasVersionVar) {
        $ldflags += " -X main.version=$Version"
        Write-Host "已注入版本号：main.version=$Version"
    } else {
        Write-Host "未找到 var version，跳过版本注入" -ForegroundColor Yellow
    }

    $exePath = Join-Path $StagePath "$AppName.exe"
    Write-Host "编译中..."
    & go build -trimpath -ldflags $ldflags -o $exePath $MainPackage
    if ($LASTEXITCODE -ne 0) { throw "go build 失败（exit $LASTEXITCODE）" }

    # 附带文件：存在才复制
    $extraFiles = @("README.md", "LICENSE", "start.bat", "stop.bat")
    foreach ($name in $extraFiles) {
        $src = Join-Path $RepoRoot $name
        if (Test-Path $src) {
            Copy-Item -LiteralPath $src -Destination (Join-Path $StagePath $name)
        }
    }

    if (Test-Path $ZipPath) { Remove-Item $ZipPath -Force }
    Write-Host "打包中..."
    Compress-Archive -Path (Join-Path $StagePath "*") -DestinationPath $ZipPath -CompressionLevel Optimal

    $hash = (Get-FileHash -LiteralPath $ZipPath -Algorithm SHA256).Hash.ToLower()
    $sumsPath = Join-Path $OutputPath "SHA256SUMS"
    "$hash *$ZipName" | Set-Content -LiteralPath $sumsPath -Encoding ASCII

    Write-Host ""
    Write-Host "完成：" -ForegroundColor Green
    Write-Host ("  产物：{0}（{1:N1} MB）" -f $ZipPath, ((Get-Item $ZipPath).Length / 1MB))
    Write-Host ("  SHA256：{0}" -f $hash)
    Write-Host ("  校验文件：{0}" -f $sumsPath)
} finally {
    Pop-Location
}
