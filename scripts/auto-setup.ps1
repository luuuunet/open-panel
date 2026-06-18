# OWPanel — 一键自动搭建（Windows）
# Usage (PowerShell):
#   .\scripts\auto-setup.ps1                    # 构建
#   .\scripts\auto-setup.ps1 -Action build      # 构建前端 + Linux 发布包
#   .\scripts\auto-setup.ps1 -Action install    # 本机安装（管理员）
#   .\scripts\auto-setup.ps1 -Action deploy     # 构建并部署到远程
#   .\scripts\auto-setup.ps1 -Action dev        # 仅启动后端（前端另开终端 npm run dev）
#   .\scripts\auto-setup.ps1 -Action docker     # Docker Compose

param(
    [ValidateSet('build', 'install', 'deploy', 'dev', 'docker')]
    [string]$Action = 'build',
    [string]$GoArch = $env:DEPLOY_GOARCH
)

if (-not $GoArch) { $GoArch = 'amd64' }

$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
$Dist = Join-Path $Root 'dist\owpanel-linux-amd64'

function Write-Log($msg) { Write-Host "[auto-setup] $msg" }

function Build-Frontend {
    Write-Log '构建前端...'
    Push-Location (Join-Path $Root 'frontend')
    if (Test-Path 'package-lock.json') { npm ci } else { npm install }
    npm run build
    Pop-Location
}

function Build-BackendLinux {
    Write-Log "交叉编译 Linux/$GoArch 后端..."
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw '未检测到 Go，请先安装: winget install GoLang.Go'
    }
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    Push-Location (Join-Path $Root 'backend')
    $env:GOOS = 'linux'
    $env:GOARCH = $GoArch
    $env:CGO_ENABLED = '0'
    go build -ldflags='-s -w' -o (Join-Path $Dist 'owpanel') .\cmd\server\
    go build -ldflags='-s -w' -o (Join-Path $Dist 'op') .\cmd\op\
    Pop-Location
    $webSrc = Join-Path $Root 'backend\web'
    $webDst = Join-Path $Dist 'web'
    if (Test-Path $webDst) { Remove-Item -Recurse -Force $webDst }
    Copy-Item -Recurse -Force $webSrc $webDst
    Write-Log "发布包: $Dist"
}

function Invoke-Deploy {
    $envFile = Join-Path $Root 'scripts\deploy.env'
    if (-not (Test-Path $envFile)) {
        throw '请先复制 scripts\deploy.env.example 为 scripts\deploy.env 并填写服务器信息'
    }
    Build-Frontend
    Build-BackendLinux
    $py = Get-Command python -ErrorAction SilentlyContinue
    if (-not $py) { $py = Get-Command python3 -ErrorAction SilentlyContinue }
    if (-not $py) { throw '需要 Python 3 运行 deploy.py' }
    & $py.Source -c 'import paramiko' 2>$null
    if ($LASTEXITCODE -ne 0) { throw '请安装 paramiko: pip install paramiko' }
    & $py.Source (Join-Path $Root 'scripts\deploy.py') --full --binary (Join-Path $Dist 'owpanel')
}

function Invoke-Dev {
    Push-Location (Join-Path $Root 'frontend')
    if (Test-Path 'package-lock.json') { npm ci } else { npm install }
    Pop-Location
    Push-Location (Join-Path $Root 'backend')
    go mod download
    Write-Log '请在另一终端运行: cd frontend; npm run dev'
    Write-Log '启动后端...'
    go run .\cmd\server\
}

Write-Host '========================================='
Write-Host '  OWPanel 自动搭建'
Write-Host "  模式: $Action"
Write-Host '========================================='

switch ($Action) {
    'build' {
        Build-Frontend
        Build-BackendLinux
        Write-Log '构建完成'
    }
    'install' {
        & (Join-Path $Root 'scripts\install.ps1') -FromSource
    }
    'deploy' {
        Invoke-Deploy
    }
    'dev' {
        Invoke-Dev
    }
    'docker' {
        Push-Location $Root
        docker compose up -d --build
        Pop-Location
        Write-Log 'Docker 已启动: http://127.0.0.1:8888'
    }
}

Write-Host '========================================='
