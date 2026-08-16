# Exports the Godot client to WebAssembly and drops it where the Go server
# serves it from.
#
# Requires Godot 4.3+ AND the matching export templates. Templates are a
# separate download from the editor itself: Editor > Manage Export Templates.
# Without them the export fails with "No export template found".

param(
    # Path to the Godot editor binary. On PATH as `godot` for many installs;
    # otherwise pass the full path to Godot_v4.x-stable_win64.exe.
    [string]$Godot = "godot",

    [string]$Preset = "Web",

    # Debug keeps stack traces and the remote debugger; release is what you
    # deploy. Both produce the same file layout.
    [switch]$Debug
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$projectDir = Join-Path $repoRoot "client"
$outDir = Join-Path $repoRoot "build\web"
$outFile = Join-Path $outDir "index.html"

if (-not (Get-Command $Godot -ErrorAction SilentlyContinue)) {
    throw "Godot no encontrado como '$Godot'. Pasalo con -Godot 'C:\ruta\a\Godot.exe'"
}

# Godot will not create the output directory itself.
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

# Stale files from an older export would be served alongside the new ones.
Get-ChildItem $outDir -File -Exclude ".gitkeep" | Remove-Item -Force

$exportFlag = if ($Debug) { "--export-debug" } else { "--export-release" }

Write-Host "Exportando '$Preset' a $outDir ..."
& $Godot --headless --path $projectDir $exportFlag $Preset $outFile

if ($LASTEXITCODE -ne 0) {
    throw "El export fallo (exit $LASTEXITCODE). Si dice 'No export template found', instalalos desde Editor > Manage Export Templates."
}

if (-not (Test-Path $outFile)) {
    throw "El export termino OK pero no aparecio $outFile"
}

Write-Host ""
Write-Host "Listo. Para servirlo:"
Write-Host "  go run -C $repoRoot\server ./cmd/server -web-dir $outDir"
Write-Host "  y abri http://localhost:8080"
Get-ChildItem $outDir -File | Select-Object Name, Length | Format-Table -AutoSize
