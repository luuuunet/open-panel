# OWPanel — Windows installer
# Usage (Admin PowerShell):
#   Set-ExecutionPolicy Bypass -Scope Process -Force
#   .\scripts\install.ps1
#   .\scripts\install.ps1 -InstallDir C:\owpanel -Port 8888 -FromSource

param(
    [string]$InstallDir = "C:\owpanel",
    [int]$Port = 8888,
    [switch]$FromSource,
    [string]$RepoUrl = "https://github.com/luuuunet/owpanel.git",
    [string]$Branch = "main"
)

$ErrorActionPreference = "Stop"

function Write-Log($msg) { Write-Host "[owpanel] $msg" }

function Require-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    $p = New-Object Security.Principal.WindowsPrincipal($id)
    if (-not $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "请以管理员身份运行 PowerShell"
    }
}

function Ensure-Dir($path) {
    if (-not (Test-Path $path)) { New-Item -ItemType Directory -Path $path -Force | Out-Null }
}

function Build-FromSource {
    param([string]$Root, [string]$OutDir)
    Write-Log "从源码构建..."
    $work = Join-Path $env:TEMP "owpanel-src-$(Get-Random)"
    git clone --depth 1 -b $Branch $RepoUrl $work
    Push-Location (Join-Path $work "backend")
    go mod download
    go build -ldflags="-s -w" -o (Join-Path $OutDir "owpanel.exe") .\cmd\server\
    go build -ldflags="-s -w" -o (Join-Path $OutDir "op.exe") .\cmd\op\
    Pop-Location
    Push-Location (Join-Path $work "frontend")
    if (Get-Command npm -ErrorAction SilentlyContinue) {
        npm ci
        npm run build
    }
    Pop-Location
    $webSrc = Join-Path $work "backend\web"
    if (Test-Path $webSrc) {
        Copy-Item -Recurse -Force $webSrc (Join-Path $OutDir "web")
    }
    Remove-Item -Recurse -Force $work
}

function Install-ScheduledTask {
    param([string]$Exe, [string]$DataDir, [string]$WebDir, [int]$PanelPort)
    $taskName = "OWPanel"
    $workDir = Split-Path $Exe
    $wrapper = Join-Path $workDir "owpanel-start.ps1"
    $wrapperContent = @"
`$env:OWPANEL_PORT='$PanelPort'
`$env:OWPANEL_HOME='$workDir'
`$env:OWPANEL_DATA='$DataDir'
`$env:OWPANEL_WEB='$WebDir'
& '$Exe'
"@
    Set-Content -Path $wrapper -Value $wrapperContent -Encoding UTF8
    $action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$wrapper`"" -WorkingDirectory $workDir
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -RunLevel Highest -Description "OWPanel server" | Out-Null
    Write-Log "已注册计划任务: $taskName（开机自启，环境变量见 $wrapper）"
    Start-ScheduledTask -TaskName $taskName
}

function Open-FirewallPort([int]$PanelPort) {
    $ruleName = "OWPanel $PanelPort"
    if (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue) { return }
    New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Protocol TCP -LocalPort $PanelPort -Action Allow | Out-Null
    Write-Log "已开放防火墙端口 $PanelPort"
}

Write-Host "========================================="
Write-Host "  OWPanel 多系统安装 (Windows)"
Write-Host "========================================="
Require-Admin

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Log "未检测到 Go，请先安装: winget install GoLang.Go"
    throw "Go 未安装"
}

Ensure-Dir $InstallDir
Ensure-Dir (Join-Path $InstallDir "data")
Ensure-Dir (Join-Path $InstallDir "logs")

$exe = Join-Path $InstallDir "owpanel.exe"
$webDir = Join-Path $InstallDir "web"
$dataDir = Join-Path $InstallDir "data"

if ($FromSource -or -not (Test-Path $exe)) {
    Build-FromSource -Root $InstallDir -OutDir $InstallDir
} else {
    Write-Log "使用已有二进制: $exe"
}

if (-not (Test-Path $webDir)) {
    throw "缺少 web 静态文件目录: $webDir"
}

Install-ScheduledTask -Exe $exe -DataDir $dataDir -WebDir $webDir -PanelPort $Port
Open-FirewallPort -PanelPort $Port

$opLink = Join-Path $InstallDir "op.exe"
if (Test-Path $opLink) {
    Write-Log "CLI: $opLink"
}

Write-Host ""
Write-Host "========================================="
Write-Host "  安装完成"
Write-Host "  地址: http://127.0.0.1:$Port"
Write-Host "  账号: admin / (随机密码，见下方文件)"
Write-Host "  密码文件: $dataDir\INITIAL_CREDENTIALS.txt"
Write-Host "  或查看服务日志获取首次登录密码"
Write-Host "========================================="
