param(
    [string]$SourceRepo = 'E:\Desktop\game\mir2x',
    [string]$LayoutFile = '',
    [string]$OutputFile = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..')
if ([string]::IsNullOrWhiteSpace($LayoutFile)) {
    $LayoutFile = Join-Path $repoRoot 'internal/protocol/testdata/cpp-layout.json'
}
if ([string]::IsNullOrWhiteSpace($OutputFile)) {
    $OutputFile = Join-Path $repoRoot 'internal/protocol/messages_generated.go'
}
$SourceRepo = (Resolve-Path -LiteralPath $SourceRepo).Path
$LayoutFile = (Resolve-Path -LiteralPath $LayoutFile).Path
$layout = Get-Content -LiteralPath $LayoutFile -Raw | ConvertFrom-Json
$utf8 = New-Object System.Text.UTF8Encoding($false)

function Parse-Enum([string]$body, [string]$label) {
    $clean = [regex]::Replace($body, '(?m)//.*$', '')
    $parts = $clean -split ','
    $result = New-Object System.Collections.Generic.List[object]
    $lookup = @{}
    $next = 0
    foreach ($rawPart in $parts) {
        $part = $rawPart.Trim()
        if ([string]::IsNullOrWhiteSpace($part)) { continue }
        $match = [regex]::Match($part, '^([A-Za-z_][A-Za-z0-9_]*)\s*(?:=\s*(.+))?$')
        if (-not $match.Success) {
            throw "cannot parse $label enum entry: $part"
        }
        $name = $match.Groups[1].Value
        if ($match.Groups[2].Success) {
            $expr = $match.Groups[2].Value.Trim()
            if ($expr -match '^\d+$') {
                $value = [int]$expr
            }
            elseif ($lookup.ContainsKey($expr)) {
                $value = [int]$lookup[$expr]
            }
            else {
                throw "cannot evaluate $label enum expression $expr for $name"
            }
        }
        else {
            $value = $next
        }
        $lookup[$name] = $value
        $next = $value + 1
        $result.Add([pscustomobject]@{ Name = $name; Value = $value })
    }
    return $result
}

$clientText = Get-Content -LiteralPath (Join-Path $SourceRepo 'common/src/clientmsg.hpp') -Raw
$serverText = Get-Content -LiteralPath (Join-Path $SourceRepo 'common/src/servermsg.hpp') -Raw

$clientEnumBody = [regex]::Match($clientText, '(?s)enum CMType\s*:\s*uint8_t\s*\{(.*?)\};').Groups[1].Value
$serverEnumBody = [regex]::Match($serverText, '(?s)enum SMType\s*:\s*uint8_t\s*\{(.*?)\};').Groups[1].Value
$clientEnum = Parse-Enum $clientEnumBody 'CMType'
$serverEnum = Parse-Enum $serverEnumBody 'SMType'

function Parse-Registry([string]$text, [string]$macro, [string]$enumPrefix, $enumEntries) {
    $pattern = "_RSVD_register_$macro\s*\(\s*($enumPrefix[A-Za-z0-9_]+)\s*,\s*([0-3])\s*(?:,\s*sizeof\s*\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)\s*)?\)"
    $matches = [regex]::Matches($text, $pattern)
    $enumValues = @{}
    foreach ($entry in $enumEntries) {
        $enumValues[$entry.Name] = [int]$entry.Value
    }
    $registeredByName = @{}
    $registeredByValue = @{}
    foreach ($match in $matches) {
        $name = $match.Groups[1].Value
        $kind = [int]$match.Groups[2].Value
        $cppType = if ($match.Groups[3].Success) { $match.Groups[3].Value } else { '' }
        if (-not $enumValues.ContainsKey($name)) {
            throw "registered message $name is not present in $enumPrefix enum"
        }
        if ($registeredByName.ContainsKey($name)) {
            throw "duplicate registration for $name"
        }
        if ($kind -in @(1, 2)) {
            if ([string]::IsNullOrWhiteSpace($cppType)) {
                throw "$name has fixed-size kind $kind but no sizeof()"
            }
            $sizeProperty = $layout.struct_sizes.PSObject.Properties[$cppType]
            if ($null -eq $sizeProperty) {
                throw "layout file has no size for $cppType used by $name"
            }
            $dataLen = [int]$sizeProperty.Value
        }
        else {
            $dataLen = 0
        }
        $reg = [pscustomobject]@{
            Name = $name
            Value = [int]$enumValues[$name]
            Kind = $kind
            CppType = $cppType
            DataLen = $dataLen
        }
        $registeredByName[$name] = $reg
        if ($registeredByValue.ContainsKey($reg.Value)) {
            throw "duplicate registration for value $($reg.Value): $($registeredByValue[$reg.Value].Name) and $name"
        }
        $registeredByValue[$reg.Value] = $reg
    }
    if ($registeredByName.Count -eq 0) {
        throw "no $macro registrations found"
    }

    $rows = New-Object System.Collections.Generic.List[object]
    foreach ($entry in $enumEntries) {
        $value = [int]$entry.Value
        if ($value -eq 0 -or $entry.Name.EndsWith('_END')) {
            $rows.Add([pscustomobject]@{
                Name = $entry.Name
                Value = $value
                Kind = 0
                CppType = ''
                DataLen = 0
                Primary = ($entry.Name -eq ($enumPrefix + 'NONE_0'))
            })
            continue
        }
        if (-not $registeredByValue.ContainsKey($value)) {
            throw "enum entry $($entry.Name) with value $value has no message registration"
        }
        $reg = $registeredByValue[$value]
        $rows.Add([pscustomobject]@{
            Name = $entry.Name
            Value = $value
            Kind = $reg.Kind
            CppType = $reg.CppType
            DataLen = $reg.DataLen
            Primary = ($reg.Name -eq $entry.Name)
        })
    }
    return $rows
}

$clientRows = Parse-Registry $clientText 'clientmsg' 'CM_' $clientEnum
$serverRows = Parse-Registry $serverText 'servermsg' 'SM_' $serverEnum

function Kind-Name([int]$kind) {
    switch ($kind) {
        0 { return 'MessageEmpty' }
        1 { return 'MessageFixedCompressed' }
        2 { return 'MessageFixedRaw' }
        3 { return 'MessageVariable' }
        default { throw "unknown message kind $kind" }
    }
}

$builder = New-Object System.Text.StringBuilder
[void]$builder.AppendLine('// Code generated by tools/protocol-fixtures/generate-message-table.ps1; DO NOT EDIT.')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('package protocol')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('type CMType uint8')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('const (')
foreach ($row in $clientRows) {
    [void]$builder.AppendLine(('    {0} CMType = {1}' -f $row.Name, $row.Value))
}
[void]$builder.AppendLine(')')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('type SMType uint8')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('const (')
foreach ($row in $serverRows) {
    [void]$builder.AppendLine(('    {0} SMType = {1}' -f $row.Name, $row.Value))
}
[void]$builder.AppendLine(')')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('var clientMessageAttributes = map[CMType]MessageAttribute{')
foreach ($row in $clientRows) {
    if (-not $row.Primary) { continue }
    [void]$builder.AppendLine(('    {0}: {{Name: "{0}", Kind: {1}, DataLen: {2}}},' -f $row.Name, (Kind-Name $row.Kind), $row.DataLen))
}
[void]$builder.AppendLine('}')
[void]$builder.AppendLine('')
[void]$builder.AppendLine('var serverMessageAttributes = map[SMType]MessageAttribute{')
foreach ($row in $serverRows) {
    if (-not $row.Primary) { continue }
    [void]$builder.AppendLine(('    {0}: {{Name: "{0}", Kind: {1}, DataLen: {2}}},' -f $row.Name, (Kind-Name $row.Kind), $row.DataLen))
}
[void]$builder.AppendLine('}')

$parent = Split-Path -Parent $OutputFile
if ($parent) { New-Item -ItemType Directory -Path $parent -Force | Out-Null }
[System.IO.File]::WriteAllText($OutputFile, $builder.ToString(), $utf8)
Write-Host "Wrote $($clientRows.Count) client and $($serverRows.Count) server messages to $OutputFile"
