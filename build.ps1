$today = Get-Date -Format "yyyyMMdd"
$branch = git branch --show-current
$suffix = ""
if ($branch -ne "main") {
    $suffix = "-$branch"
}
$version = "v2.$today.1$suffix"
$ldflags = "-X DockSTARTer2/internal/version.Version=$version"

Write-Host "Building DockSTARTer2 version $version..." -ForegroundColor Cyan
go build -ldflags "$ldflags" -o ds2.exe

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful: ds2.exe" -ForegroundColor Green
}
else {
    Write-Host "Build failed!" -ForegroundColor Red
}
