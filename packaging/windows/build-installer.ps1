param(
    [string]$Version = "",
    [switch]$SkipTests,
    [switch]$RegenAssets,
    [switch]$SkipSign,
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $wbGo = "C:\Users\xiaotao2026\.workbuddy\binaries\go\versions\1.25.1\go\bin"
    if (Test-Path $wbGo) {
        $env:Path = "$wbGo;$env:Path"
    }
}

$pkgDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent (Split-Path -Parent $pkgDir)

Push-Location $root
try {
    if (-not $Version) {
        $mainGo = Get-Content (Join-Path $root "cmd\memora\main.go") -Raw
        if ($mainGo -match 'const\s+version\s*=\s*"([^"]+)"') {
            $Version = $Matches[1]
        } else {
            throw "Could not detect version from cmd\memora\main.go"
        }
    }

    $needAssets = $RegenAssets -or
        (-not (Test-Path (Join-Path $root "assets\brand\memora.ico"))) -or
        (-not (Test-Path (Join-Path $root "cmd\memora\memora_tray.png"))) -or
        (-not (Test-Path (Join-Path $pkgDir "wizard.bmp"))) -or
        (-not (Test-Path (Join-Path $pkgDir "header.bmp")))
    if ($needAssets) {
        go run .\packaging\windows\mkassets.go
        if ($LASTEXITCODE -ne 0) { throw "mkassets.go failed" }
    }

    $sysoPath = Join-Path $root "cmd\memora\rsrc_windows_amd64.syso"
    if ($RegenAssets -or (-not (Test-Path $sysoPath))) {
        go run .\packaging\windows\gen-syso.go
        if ($LASTEXITCODE -ne 0) { throw "gen-syso.go failed" }
    }

    if (-not $SkipTests) {
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
        go test -count=1 ./...
        if ($LASTEXITCODE -ne 0) { throw "go test failed" }
    }

    $env:CGO_ENABLED = "0"
    $binExe = Join-Path $root "bin\memora.exe"
    if (-not $SkipBuild) {
        go build -trimpath -o $binExe ./cmd/memora
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } elseif (-not (Test-Path $binExe)) {
        throw "SkipBuild set but $binExe does not exist"
    }

    $signScript = Join-Path $pkgDir "sign-authenticode.ps1"
    if (-not $SkipSign -and (Test-Path $signScript)) {
        & $signScript -Files @($binExe)
    }

    $makensis = (Get-Command "makensis.exe" -ErrorAction SilentlyContinue).Source
    if (-not $makensis) {
        foreach ($cand in @(
            "C:\Program Files (x86)\NSIS\makensis.exe",
            "C:\Program Files\NSIS\makensis.exe"
        )) {
            if (Test-Path $cand) {
                $makensis = $cand
                break
            }
        }
    }
    if (-not $makensis) {
        throw "NSIS (makensis.exe) not found. Please install NSIS 3.x."
    }

    $releaseDir = Join-Path $root "bin\release"
    New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

    $notesFile = Join-Path $releaseDir ("RELEASE_NOTES-v{0}.md" -f $Version)
    if (-not (Test-Path $notesFile)) {
        $notesFile = Join-Path $root "README.md"
    }

    $setupName = "memora-v{0}-windows-amd64-setup.exe" -f $Version
    $setupOut = Join-Path $releaseDir $setupName

    $nsisArgs = @(
        "/INPUTCHARSET", "UTF8",
        "/DAPP_VERSION=$Version",
        "/DROOT_DIR=$root",
        "/DBIN_EXE=$binExe",
        "/DRELEASE_NOTES=$notesFile",
        "/DOUTFILE=$setupOut"
    )
    if ($SkipSign) {
        $nsisArgs += "/DSKIP_SIGN=1"
    }
    $nsisArgs += (Join-Path $pkgDir "memora.nsi")
    & $makensis @nsisArgs
    if ($LASTEXITCODE -ne 0) { throw "makensis failed with exit code $LASTEXITCODE" }

    if (-not $SkipSign -and (Test-Path $signScript)) {
        & $signScript -Files @($setupOut)
    }

    $portableOut = Join-Path $releaseDir "memora.exe"
    Copy-Item -Path $binExe -Destination $portableOut -Force
    Unblock-File -Path $portableOut -ErrorAction SilentlyContinue

    $cerOut = Join-Path $releaseDir "MEMORA-CodeSign.cer"
    $setupHash = (Get-FileHash $setupOut -Algorithm SHA256).Hash.ToLower()
    $exeHash = (Get-FileHash $portableOut -Algorithm SHA256).Hash.ToLower()
    $sumFile = Join-Path $releaseDir ("SHA256SUMS-v{0}.txt" -f $Version)
    $sumLines = @(
        "$setupHash  $setupName",
        "$exeHash  memora.exe"
    )
    if (Test-Path $cerOut) {
        $cerHash = (Get-FileHash $cerOut -Algorithm SHA256).Hash.ToLower()
        $sumLines += "$cerHash  MEMORA-CodeSign.cer"
    }
    $sumLines | Set-Content -Path $sumFile -Encoding ascii

    $setupItem = Get-Item -LiteralPath $setupOut
    "built installer: {0} ({1:N0} bytes)" -f $setupItem.FullName, $setupItem.Length
    "sha256sums: {0}" -f $sumFile
}
finally {
    Pop-Location
}
