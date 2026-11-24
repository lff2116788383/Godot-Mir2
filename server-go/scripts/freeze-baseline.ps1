param(
    [string]$SourceRepo = 'E:\Desktop\game\mir2x',
    [string]$OutputDir = ''
)

$ErrorActionPreference = 'Stop'

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $repoRoot 'baseline'
}

$SourceRepo = (Resolve-Path -LiteralPath $SourceRepo).Path
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

$utf8 = New-Object System.Text.UTF8Encoding($false)
function Write-Utf8([string]$path, [string]$value) {
    [System.IO.File]::WriteAllText($path, $value.TrimEnd() + "`n", $utf8)
}

function Invoke-GitText([string[]]$arguments) {
    $output = & git -C $SourceRepo @arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git $($arguments -join ' ') failed:`n$($output -join "`n")"
    }
    return (($output | Out-String).TrimEnd() + "`n")
}

$commit = (& git -C $SourceRepo rev-parse HEAD).Trim()
$branch = (& git -C $SourceRepo branch --show-current).Trim()
$commitDetails = (& git -C $SourceRepo show -s --date=iso-strict --format='%H%n%an <%ae>%n%ad%n%s' HEAD | Out-String).TrimEnd()
$status = Invoke-GitText @('status', '--short', '--branch', '--untracked-files=all')

Write-Utf8 (Join-Path $OutputDir 'source.path.txt') $SourceRepo
Write-Utf8 (Join-Path $OutputDir 'source.commit.txt') @"
branch: $branch
commit: $commit

$commitDetails
"@
Write-Utf8 (Join-Path $OutputDir 'source.status.txt') $status

$patchPath = Join-Path $OutputDir 'source.patch'
& git -C $SourceRepo diff --binary "--output=$patchPath"
if ($LASTEXITCODE -ne 0) { throw 'failed to write source.patch' }

$untrackedLines = @($status -split "`r?`n" | Where-Object { $_ -match '^\?\?\s+' } | ForEach-Object { $_ -replace '^\?\?\s+', '' })
Write-Utf8 (Join-Path $OutputDir 'source.untracked.txt') (($untrackedLines | Sort-Object) -join "`n")

$trackedRoots = @(
    'server',
    'common',
    'CMakeLists.txt',
    'CMakePresets.json',
    'vcpkg.json',
    'vcpkg-configuration.json',
    'README.md'
)
$files = foreach ($entry in $trackedRoots) {
    $full = Join-Path $SourceRepo $entry
    if (Test-Path -LiteralPath $full -PathType Container) {
        Get-ChildItem -LiteralPath $full -Recurse -File
    }
    elseif (Test-Path -LiteralPath $full -PathType Leaf) {
        Get-Item -LiteralPath $full
    }
}

$hashLines = foreach ($file in ($files | Sort-Object FullName -Unique)) {
    $relative = $file.FullName.Substring($SourceRepo.Length).TrimStart('\', '/') -replace '\\', '/'
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $relative"
}
Write-Utf8 (Join-Path $OutputDir 'source-files.sha256') (($hashLines | Sort-Object) -join "`n")

$assetCandidates = @(
    'E:\Desktop\game\mir2x_assets',
    'E:\Desktop\game\mir2x_assets-main',
    'E:\Desktop\game\mir2x_data',
    (Join-Path $SourceRepo 'assets'),
    (Join-Path (Split-Path -Parent $SourceRepo) 'mir2x_assets')
)
$assetLines = foreach ($candidate in ($assetCandidates | Select-Object -Unique)) {
    if (Test-Path -LiteralPath $candidate) {
        $assetFiles = @(Get-ChildItem -LiteralPath $candidate -Recurse -File)
        $bytes = ($assetFiles | Measure-Object -Property Length -Sum).Sum
        $versionText = ''
        $versionFile = Join-Path $candidate 'version'
        if (Test-Path -LiteralPath $versionFile -PathType Leaf) {
            $versionText = (Get-Content -LiteralPath $versionFile -Raw).Trim()
        }
        $gitCommit = ''
        $gitDirty = ''
        if (Test-Path -LiteralPath (Join-Path $candidate '.git')) {
            $gitCommit = (& git -C $candidate rev-parse HEAD 2>$null).Trim()
            $gitDirtyCount = @(& git -C $candidate status --porcelain 2>$null).Count
            $gitDirty = ($gitDirtyCount -ne 0)
        }
        "FOUND`t$candidate`tfiles=$($assetFiles.Count)`tbytes=$bytes`tversion=$versionText`tgit_commit=$gitCommit`tgit_dirty=$gitDirty"
    }
    else {
        "MISSING`t$candidate"
    }
}
Write-Utf8 (Join-Path $OutputDir 'asset-status.txt') (($assetLines -join "`n"))

$summary = @(
    "source=$SourceRepo",
    "branch=$branch",
    "commit=$commit",
    "tracked_files=$(@($files).Count)",
    "untracked_entries=$($untrackedLines.Count)",
    "patch_bytes=$((Get-Item -LiteralPath $patchPath).Length)"
)
Write-Utf8 (Join-Path $OutputDir 'freeze-summary.txt') (($summary -join "`n"))

Write-Host "Baseline frozen at $OutputDir"
Write-Host "commit=$commit tracked_files=$(@($files).Count) untracked_entries=$($untrackedLines.Count)"
