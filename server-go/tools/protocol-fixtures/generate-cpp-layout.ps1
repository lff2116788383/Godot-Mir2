param(
    [string]$SourceRepo = 'E:\Desktop\game\mir2x',
    [string]$Compiler = 'E:\Desktop\game\toolchain\msys64\ucrt64\bin\g++.exe',
    [string]$OutputFile = ''
)

$ErrorActionPreference = 'Stop'

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..')
if ([string]::IsNullOrWhiteSpace($OutputFile)) {
    $OutputFile = Join-Path $repoRoot 'internal/protocol/testdata/cpp-layout.json'
}
$SourceRepo = (Resolve-Path -LiteralPath $SourceRepo).Path
$Compiler = (Resolve-Path -LiteralPath $Compiler).Path
$common = Join-Path $SourceRepo 'common/src'
$utf8 = New-Object System.Text.UTF8Encoding($false)

function Read-Source([string]$name) {
    Get-Content -LiteralPath (Join-Path $common $name) -Raw
}

function Get-Block([string]$text, [string]$startPattern, [string]$endPattern, [string]$what) {
    $match = [regex]::Match($text, "(?s)$startPattern(.*?)$endPattern")
    if (-not $match.Success) {
        throw "failed to extract $what"
    }
    return $match.Groups[1].Value
}

$clientText = Read-Source 'clientmsg.hpp'
$serverText = Read-Source 'servermsg.hpp'
$actionText = Read-Source 'actionnode.hpp'
$sysconstText = Read-Source 'sysconst.hpp'
$protocolText = Read-Source 'protocoldef.hpp'

$clientEnum = Get-Block $clientText 'enum CMType\s*:\s*uint8_t\s*\{' '\};' 'CMType'
$serverEnum = Get-Block $serverText 'enum SMType\s*:\s*uint8_t\s*\{' '\};' 'SMType'
$actionEnum = Get-Block $actionText 'enum ActionType\s*:\s*int\s*\{' '\};' 'ActionType'
$clientStructs = Get-Block $clientText '#pragma pack\(push, 1\)' '#pragma pack\(pop\)' 'client message structs'
$serverStructs = Get-Block $serverText '#pragma pack\(push, 1\)' '#pragma pack\(pop\)' 'server message structs'
$actionStruct = Get-Block $actionText '#pragma pack\(push, 1\)' '#pragma pack\(pop\)' 'ActionNode'

function Get-ConstInt([string]$text, [string]$name) {
    $match = [regex]::Match($text, "(?m)^\s*(?:constexpr|inline\s+constexpr)\s+(?:int|size_t)\s+$name\s*=\s*(\d+)")
    if (-not $match.Success) {
        throw "failed to find numeric constant $name"
    }
    return [int]$match.Groups[1].Value
}

$constants = @(
    'SYS_IDSIZE', 'SYS_PWDSIZE', 'SYS_NAMESIZE', 'SYS_DELIVERYRECORDSIZE',
    'SYS_AUCTIONNOTESIZE', 'SYS_STALLNAMESIZE', 'SYS_GUILDTITLESIZE',
    'SYS_GUILDNOTICESIZE', 'SYS_DEFSPEED'
)
$constantLines = foreach ($name in $constants) {
    $value = Get-ConstInt $sysconstText $name
    "constexpr size_t $name = $value;"
}
$dirNoneMatch = [regex]::Match($protocolText, 'DIR_NONE\s*=\s*(\d+)')
if (-not $dirNoneMatch.Success) { throw 'failed to find DIR_NONE' }
$dirNone = [int]$dirNoneMatch.Groups[1].Value
$constantLines += "constexpr int DIR_NONE = $dirNone;"

$structNames = [regex]::Matches(($clientStructs + "`n" + $serverStructs + "`n" + $actionStruct), '(?m)^struct\s+(\w+)') |
    ForEach-Object { $_.Groups[1].Value } |
    Sort-Object -Unique

if ($structNames.Count -eq 0) {
    throw 'no message structs extracted'
}

$printStructSizes = ($structNames | ForEach-Object {
    '    std::printf("SIZE\t' + $_ + '\t%zu\n", sizeof(' + $_ + '));'
}) -join "`n"

$templatePath = Join-Path $PSScriptRoot 'layout_probe.template.cpp'
$probe = Get-Content -LiteralPath $templatePath -Raw
$probe = $probe.Replace('/*__CONSTANTS__*/', ($constantLines -join "`n"))
$probe = $probe.Replace('/*__ACTION_ENUM__*/', $actionEnum)
$probe = $probe.Replace('/*__ACTION_STRUCT__*/', $actionStruct)
$probe = $probe.Replace('/*__CLIENT_ENUM__*/', $clientEnum)
$probe = $probe.Replace('/*__CLIENT_STRUCTS__*/', $clientStructs)
$probe = $probe.Replace('/*__SERVER_ENUM__*/', $serverEnum)
$probe = $probe.Replace('/*__SERVER_STRUCTS__*/', $serverStructs)
$probe = $probe.Replace('/*__STRUCT_SIZES__*/', $printStructSizes)

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ('mir2x-protocol-layout-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$probeFile = Join-Path $tempDir 'layout_probe.cpp'
$exeFile = Join-Path $tempDir 'layout_probe.exe'
$stdoutFile = Join-Path $tempDir 'stdout.txt'
$stderrFile = Join-Path $tempDir 'stderr.txt'
[System.IO.File]::WriteAllText($probeFile, $probe, $utf8)

$compilerDir = Split-Path -Parent $Compiler
$oldPath = $env:PATH
$env:PATH = "$compilerDir;$oldPath"
try {
    $process = Start-Process -FilePath $Compiler -ArgumentList @('-std=c++23', '-O0', $probeFile, '-o', $exeFile) -Wait -PassThru -NoNewWindow -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
    if ($process.ExitCode -ne 0) {
        $compilerError = if (Test-Path $stderrFile) { Get-Content $stderrFile -Raw } else { '' }
        throw "C++ layout probe failed with exit code $($process.ExitCode):`n$compilerError"
    }

    $rawOutput = (& $exeFile 2>&1 | Out-String).TrimEnd()
    if ($LASTEXITCODE -ne 0) {
        throw "C++ layout probe execution failed with exit code $LASTEXITCODE"
    }
}
finally {
    $env:PATH = $oldPath
    Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

$sizes = [ordered]@{}
$helpers = [ordered]@{}
$actionOffsets = [ordered]@{}
$bitfields = [ordered]@{}

foreach ($line in ($rawOutput -split "`r?`n")) {
    $parts = $line -split "`t"
    switch ($parts[0]) {
        'SIZE' { $sizes[$parts[1]] = [int]$parts[2] }
        'HELPER' { $helpers[$parts[1]] = [int]$parts[2] }
        'ACTION_OFFSET' { $actionOffsets[$parts[1]] = [int]$parts[2] }
        'ACTION_BITS' { $bitfields[$parts[1]] = $parts[2] }
        'CHAT_HEADER_BITS' { $bitfields['CMChatMessageHeader.' + $parts[1]] = $parts[2] }
        'LATEST_CHAT_BITS' { $bitfields['CMRequestLatestChatMessage.' + $parts[1]] = $parts[2] }
        'ONLINE_BITS' { $bitfields['SMOnlineOK.' + $parts[1]] = $parts[2] }
        'CORECORD_BITS' { $bitfields['SMCORecord.Player.' + $parts[1]] = $parts[2] }
    }
}

$commit = (& git -C $SourceRepo rev-parse HEAD).Trim()
$result = [ordered]@{
    generated_at_utc = [DateTime]::UtcNow.ToString('o')
    source_repo = $SourceRepo
    source_commit = $commit
    compiler = (& $Compiler --version | Select-Object -First 1)
    struct_sizes = $sizes
    helper_sizes = $helpers
    action_offsets = $actionOffsets
    bitfield_probes = $bitfields
    raw_output = $rawOutput
}

$parent = Split-Path -Parent $OutputFile
if ($parent) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
$json = $result | ConvertTo-Json -Depth 8
[System.IO.File]::WriteAllText($OutputFile, $json + "`n", $utf8)
Write-Host "Wrote $($sizes.Count) struct sizes to $OutputFile"
