# Build on Windows: powershell -ExecutionPolicy Bypass -File build.ps1
$ErrorActionPreference = "Stop"
go mod tidy
go test ./...
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -ldflags "-H windowsgui -s -w" -o bin\gachi-client.exe .\cmd\client
go build -o bin\gachi-server.exe .\cmd\server
Write-Host "OK: bin\gachi-client.exe, bin\gachi-server.exe"
