param(
    [string]$SourceRepo = 'E:\Desktop\game\mir2x',
    [string]$OutputFile = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
if ([string]::IsNullOrWhiteSpace($OutputFile)) {
    $OutputFile = Join-Path $repoRoot 'migration/source-index.csv'
}
$SourceRepo = (Resolve-Path -LiteralPath $SourceRepo).Path

function Get-Batch([string]$relativePath) {
    $p = $relativePath -replace '\\', '/'
    if ($p -match '^common/src/(protocoldef|clientmsg|servermsg|serdesmsg|msgf|actormsg|actormsgpack)') { return 'B1' }
    if ($p -match '^common/src/.*\.(inc)$') { return 'B1' }
    if ($p -match '^common/src/') { return 'B1' }
    if ($p -match '^server/src/server\.sql$') { return 'B2' }
    if ($p -match '(db|mapbindb|maprecord|dropitemconfig)') { return 'B2' }
    if ($p -match '(actorpool|actormsg|actorpod|actornet|netdriver|serverpeer|servicecore|dispatcher|delaydriver|receiver|peer)') { return 'B3' }
    if ($p -match '(servermap|battleobject|monster|buff|charobject|serverobject|path|rotatecoord)') { return 'B4' }
    if ($p -match '(player|inventory|item|quest|npchar|lua|guild|auction|stall|chat|delivery|friend)') { return 'B5' }
    if ($p -match '^server/test/') { return 'B8' }
    return 'B3'
}

$files = @()
foreach ($base in @('server/src', 'server/test', 'common/src')) {
    $full = Join-Path $SourceRepo $base
    if (Test-Path -LiteralPath $full) {
        $files += Get-ChildItem -LiteralPath $full -Recurse -File | Where-Object {
            $_.Extension -in @('.cpp', '.hpp', '.h', '.cc', '.c', '.inc', '.lua', '.sql', '.fl')
        }
    }
}

$rows = foreach ($file in ($files | Sort-Object FullName -Unique)) {
    $relative = $file.FullName.Substring($SourceRepo.Length).TrimStart('\', '/') -replace '\\', '/'
    $hash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    [pscustomobject]@{
        source_path = $relative
        extension = $file.Extension.TrimStart('.').ToLowerInvariant()
        sha256 = $hash
        batch = Get-Batch $relative
        go_package = ''
        status = 'pending'
        notes = ''
    }
}

$parent = Split-Path -Parent $OutputFile
if ($parent) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
$rows | Export-Csv -LiteralPath $OutputFile -NoTypeInformation -Encoding UTF8
Write-Host "Wrote $($rows.Count) rows to $OutputFile"
