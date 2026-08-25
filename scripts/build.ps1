$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$commandDirectory = Join-Path $projectRoot "cmd\reality-converter"
$manifestPath = Join-Path $commandDirectory "reality-converter.manifest"
$resourcePath = Join-Path $commandDirectory "rsrc_windows_amd64.syso"
$distributionDirectory = Join-Path $projectRoot "dist"
$outputPath = Join-Path $distributionDirectory "reality-converter.exe"

Push-Location $projectRoot
try {
    go install github.com/akavel/rsrc@v0.10.2
    $goPath = go env GOPATH
    $resourceCompiler = Join-Path $goPath "bin\rsrc.exe"
    & $resourceCompiler -manifest $manifestPath -o $resourcePath

    go mod tidy
    go test ./...
    New-Item -ItemType Directory -Force -Path $distributionDirectory | Out-Null
    go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o $outputPath ./cmd/reality-converter

    $outputFile = Get-Item $outputPath
    $outputHash = (Get-FileHash -Algorithm SHA256 $outputPath).Hash
    Write-Output "Built: $($outputFile.FullName)"
    Write-Output "Size: $($outputFile.Length) bytes"
    Write-Output "SHA256: $outputHash"
}
finally {
    Pop-Location
}
