param(
    [string]$OutputPath,
    [int]$Samples = 10
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not $OutputPath) {
    $OutputPath = Join-Path $root "benchmarks\results\current.txt"
} elseif (-not [IO.Path]::IsPathRooted($OutputPath)) {
    $OutputPath = Join-Path $root $OutputPath
}
if ($Samples -le 0) {
    throw "benchmark: Samples must be positive"
}

$OutputPath = [IO.Path]::GetFullPath($OutputPath)
$directory = Split-Path -Parent $OutputPath
[IO.Directory]::CreateDirectory($directory) | Out-Null
$encoding = [Text.UTF8Encoding]::new($false)
[IO.File]::WriteAllText($OutputPath, "", $encoding)

Push-Location $root
try {
    for ($sample = 1; $sample -le $Samples; $sample++) {
        Write-Host "# benchmark sample $sample/$Samples"
        $lines = @(& go test -C benchmarks -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' -benchmem -count 1 ./... 2>&1)
        if ($LASTEXITCODE -ne 0) {
            $lines | Write-Output
            throw "benchmark: go test failed for sample $sample"
        }
        $text = ($lines -join [Environment]::NewLine) + [Environment]::NewLine
        [IO.File]::AppendAllText($OutputPath, $text, $encoding)
        $lines | Write-Output
    }
}
finally {
    Pop-Location
}
