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
#
# The path needs the trailing wildcard: Get-ChildItem with -Exclude and a plain
# directory path matches nothing at all, silently. This deleted zero files for
# as long as it existed, which is worse than not being here -- the export
# overwrites index.* but leaves every .gz behind, and the server prefers the .gz
# for any client that accepts gzip. That is a deploy that ships the previous
# client to every browser while the files next to it look new.
Get-ChildItem -Path (Join-Path $outDir "*") -File -Exclude ".gitkeep" | Remove-Item -Force

$exportFlag = if ($Debug) { "--export-debug" } else { "--export-release" }

Write-Host "Exportando '$Preset' a $outDir ..."
$startedAt = Get-Date
& $Godot --headless --path $projectDir $exportFlag $Preset $outFile
$exit = $LASTEXITCODE

# $LASTEXITCODE comes back empty from a non-interactive host even when Godot
# exits 0, so an empty value cannot be read as failure -- it aborted a perfectly
# good export. A number that is not zero still is one, and either way the real
# check is below: the file has to be there *and* have been written just now.
if ($null -ne $exit -and $exit -ne 0) {
    throw "El export fallo (exit $exit). Si dice 'No export template found', instalalos desde Editor > Manage Export Templates."
}

if (-not (Test-Path $outFile)) {
    throw "El export termino OK pero no aparecio $outFile"
}

if ((Get-Item $outFile).LastWriteTime -lt $startedAt) {
    throw "El export no reescribio ${outFile}: quedo el de antes, no el de ahora"
}

# Pre-compress the big files. The server serves "<file>.gz" to any client that
# accepts gzip (see transport.precompressed) and falls back to the plain file
# otherwise, so this is an optimisation and never a requirement. It matters a
# lot though: the wasm alone is 38MB raw against under 10MB gzipped, and
# http.FileServer does not compress anything on its own.
#
# Done here rather than per request because the machine serving this is a
# 256MB shared CPU — compressing 38MB for every visitor would cost far more
# than the bandwidth it saves.
Write-Host "Comprimiendo para servir..."
foreach ($file in Get-ChildItem $outDir -File -Include *.wasm, *.pck, *.js, *.html -Recurse) {
    $gz = "$($file.FullName).gz"
    $in = [System.IO.File]::OpenRead($file.FullName)
    try {
        $out = [System.IO.File]::Create($gz)
        try {
            $stream = New-Object System.IO.Compression.GZipStream($out, [System.IO.Compression.CompressionLevel]::Optimal)
            try { $in.CopyTo($stream) } finally { $stream.Dispose() }
        } finally { $out.Dispose() }
    } finally { $in.Dispose() }
    $saved = [math]::Round((1 - (Get-Item $gz).Length / $file.Length) * 100)
    Write-Host ("  {0,-28} -{1}%" -f $file.Name, $saved)
}

Write-Host ""
Write-Host "Listo. Para servirlo:"
Write-Host "  go run -C $repoRoot\server ./cmd/server -web-dir $outDir"
Write-Host "  y abri http://localhost:8080"
Get-ChildItem $outDir -File | Select-Object Name, Length | Format-Table -AutoSize
