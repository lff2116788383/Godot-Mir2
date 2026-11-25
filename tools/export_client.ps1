param(
    [string]$GodotBin = $env:GODOT_BIN,
    [string]$Output = "build/Godot-Mir2.exe"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$project = Join-Path $root "client-godot"
$outPath = Join-Path $root $Output

if ([string]::IsNullOrWhiteSpace($GodotBin)) {
    $GodotBin = "E:\Program Files\Godot\Godot_v4.7.2-stable_win64.exe"
}
if (-not (Test-Path -LiteralPath $GodotBin)) {
    throw "Godot 可执行文件不存在: $GodotBin。请通过 -GodotBin 或 GODOT_BIN 指定。"
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $outPath) | Out-Null
& $GodotBin --headless --path $project --export-release "Windows Desktop" $outPath
if ($LASTEXITCODE -ne 0) {
    throw "Godot 导出失败，exit=$LASTEXITCODE"
}

$assetRoot = if ($env:MIR2X_ASSETS) { $env:MIR2X_ASSETS } else { "E:\Desktop\game\mir2x_assets-main" }
$launcher = Join-Path (Split-Path -Parent $outPath) "run-client.ps1"
@"
`$env:MIR2X_ASSETS = '$assetRoot'
& '$outPath'
"@ | Set-Content -LiteralPath $launcher -Encoding utf8
Write-Host "导出完成: $outPath"
Write-Host "运行脚本: $launcher"
Write-Host "资源目录: $assetRoot"
