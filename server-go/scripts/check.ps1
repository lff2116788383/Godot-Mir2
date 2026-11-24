param(
    [switch]$Race
)

$ErrorActionPreference = 'Stop'
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
Push-Location $repoRoot
try {
    Write-Host '[1/4] gofmt'
    $unformatted = @(& gofmt -l . 2>&1)
    if ($LASTEXITCODE -ne 0) {
        throw 'gofmt failed'
    }
    if ($unformatted.Count -ne 0) {
        throw "unformatted files:`n$($unformatted -join "`n")"
    }

    Write-Host '[2/4] go vet'
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }

    Write-Host '[3/4] go test'
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }

    Write-Host '[4/4] go build'
    & go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }

    if ($Race) {
        Write-Host '[extra] go test -race'
        & go test -race ./...
        if ($LASTEXITCODE -ne 0) { throw 'go test -race failed' }
    }

    Write-Host 'All checks passed.'
}
finally {
    Pop-Location
}
