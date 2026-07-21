$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        $portableGit = Join-Path $env:LOCALAPPDATA "Programs\PortableGit-2.55.0.3\cmd"
        if (Test-Path -LiteralPath (Join-Path $portableGit "git.exe")) {
            $env:PATH = "$portableGit;$env:PATH"
        }
    }

    $goFiles = Get-ChildItem -Path $root -Recurse -Filter *.go -File |
        Where-Object { $_.FullName -notmatch "[\\/]vendor[\\/]" } |
        ForEach-Object { $_.FullName }
    $unformatted = if ($goFiles) { & gofmt -l $goFiles }
    if ($unformatted) {
        Write-Error "gofmt required:`n$($unformatted -join "`n")"
    }

    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

    & go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 run
    if ($LASTEXITCODE -ne 0) { throw "golangci-lint failed" }

    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed" }
}
finally {
    Pop-Location
}
