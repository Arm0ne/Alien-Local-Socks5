$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $PSScriptRoot
$converterCommandDirectory = Join-Path $projectRoot "cmd\reality-converter"
$converterManifestPath = Join-Path $converterCommandDirectory "reality-converter.manifest"
$converterResourcePath = Join-Path $converterCommandDirectory "rsrc_windows_amd64.syso"
$integratedCommandDirectory = Join-Path $projectRoot "cmd\reality-local"
$integratedManifestPath = Join-Path $integratedCommandDirectory "reality-local.manifest"
$integratedResourcePath = Join-Path $integratedCommandDirectory "rsrc_windows_amd64.syso"
$xraySourcePath = Join-Path $projectRoot ".tools\xray-v26.3.27\bin\xray.exe"
$xrayAssetDirectory = Join-Path $projectRoot "internal\xrayruntime\assets"
$xrayAssetPath = Join-Path $xrayAssetDirectory "xray.exe"
$distributionDirectory = Join-Path $projectRoot "dist"
$converterOutputPath = Join-Path $distributionDirectory "reality-converter.exe"
$integratedOutputPath = Join-Path $distributionDirectory "Reality Local.exe"

Push-Location $projectRoot
try {
    go install github.com/akavel/rsrc@v0.10.2
    $goPath = go env GOPATH
    $resourceCompiler = Join-Path $goPath "bin\rsrc.exe"
    & $resourceCompiler -manifest $converterManifestPath -o $converterResourcePath
    & $resourceCompiler -manifest $integratedManifestPath -o $integratedResourcePath

    if (-not (Test-Path -LiteralPath $xraySourcePath)) {
        throw "Missing verified Xray executable: $xraySourcePath"
    }
    $xrayHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $xraySourcePath).Hash.ToLowerInvariant()
    $expectedXrayHash = "15c2d007954ac53ba69b80ec91242786b3c0b71d52649165b4ca1d5cc96ef8f1"
    if ($xrayHash -ne $expectedXrayHash) {
        throw "Xray SHA256 mismatch. Expected $expectedXrayHash, got $xrayHash"
    }
    New-Item -ItemType Directory -Force -Path $xrayAssetDirectory | Out-Null
    Copy-Item -LiteralPath $xraySourcePath -Destination $xrayAssetPath -Force

    go mod tidy
    go test ./...
    New-Item -ItemType Directory -Force -Path $distributionDirectory | Out-Null
    go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.0.0" -o $converterOutputPath ./cmd/reality-converter
    go build -tags integrated_xray -trimpath -ldflags "-s -w -H=windowsgui -X main.version=1.1.0" -o $integratedOutputPath ./cmd/reality-local
    Copy-Item -LiteralPath (Join-Path $projectRoot "THIRD-PARTY-NOTICES.txt") -Destination $distributionDirectory -Force

    foreach ($outputPath in $converterOutputPath, $integratedOutputPath) {
        $outputFile = Get-Item $outputPath
        $outputHash = (Get-FileHash -Algorithm SHA256 $outputPath).Hash
        Write-Output "Built: $($outputFile.FullName)"
        Write-Output "Size: $($outputFile.Length) bytes"
        Write-Output "SHA256: $outputHash"
    }
}
finally {
    Pop-Location
}
