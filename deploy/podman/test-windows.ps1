<#
.SYNOPSIS
  Runs the same checks as GitLab CI on a Windows PC, using only Podman Desktop
  (no local Go, Node or Python needed).

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File deploy\podman\test-windows.ps1
  powershell -ExecutionPolicy Bypass -File deploy\podman\test-windows.ps1 -SkipSmoke
#>
param(
    [string]$Image = 'deskbook:local',
    [switch]$SkipSmoke
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path

function Step($text) { Write-Host "`n== $text" -ForegroundColor Cyan }
function Check($what) {
    if ($LASTEXITCODE -ne 0) { Write-Host "FAILED: $what" -ForegroundColor Red; exit 1 }
    Write-Host "ok - $what" -ForegroundColor Green
}

# C:\Users\me\deskbook -> /mnt/c/Users/me/deskbook (path inside the Podman machine on WSL)
function To-MachinePath([string]$winPath) {
    $drive = $winPath.Substring(0, 1).ToLower()
    return "/mnt/$drive" + ($winPath.Substring(2) -replace '\\', '/')
}

if (-not (Get-Command podman -ErrorAction SilentlyContinue)) {
    Write-Host 'Podman not found. Install Podman Desktop: winget install RedHat.Podman-Desktop' -ForegroundColor Red
    exit 1
}
podman info --format '{{.Host.Arch}}' | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Podman machine is not running. Run: podman machine init; podman machine start' -ForegroundColor Red
    exit 1
}

Step 'Python business-rule tests'
podman run --rm -e PYTHONDONTWRITEBYTECODE=1 -v "$root\web\src\python:/src" -w /src `
    docker.io/library/python:3.12-slim python -m unittest discover -s tests -v
Check 'Python tests'

Step 'Go vet + tests (race detector)'
podman run --rm -v "$root\server:/src" -w /src docker.io/library/golang:1.24-bookworm `
    sh -c 'go vet ./... && go test -race -count=1 ./...'
Check 'Go tests'

Step "Container image build (frontend build + offline check run inside) -> $Image"
podman build -t $Image -f "$root\Containerfile" $root
Check 'image build'

if ($SkipSmoke) { Write-Host "`nSmoke test skipped."; exit 0 }

Step 'Smoke test inside the Podman machine'
$script = To-MachinePath (Join-Path $root 'deploy\podman\smoke-test.sh')
podman machine ssh "bash '$script' '$Image'"
Check 'smoke test'

Write-Host "`nAll checks passed. Try it: podman run --rm -p 8080:8080 -e DESKBOOK_ADMIN_PASSWORD=change-me-now-please $Image" -ForegroundColor Green
Write-Host 'then open http://localhost:8080 and sign in as admin.'
