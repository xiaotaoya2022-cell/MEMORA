param(
    [string]$Output = "bin\memora.exe",
    [switch]$Installer,
    [switch]$SkipTests,
    [switch]$SkipSign
)

$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "0"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $wbGo = "C:\Users\xiaotao2026\.workbuddy\binaries\go\versions\1.25.1\go\bin"
    if (Test-Path $wbGo) {
        $env:Path = "$wbGo;$env:Path"
    }
}

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $root
try {
    go build -trimpath -o $Output ./cmd/memora
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    $signScript = Join-Path $root "packaging\windows\sign-authenticode.ps1"
    $fullOut = Join-Path $root $Output
    if (-not $SkipSign -and (Test-Path $signScript)) {
        & $signScript -Files @($fullOut)
    }
    $item = Get-Item -LiteralPath $fullOut
    "built: {0} ({1:N0} bytes)" -f $item.FullName, $item.Length

    if ($Installer) {
        $args = @{}
        if ($SkipTests) { $args["SkipTests"] = $true }
        if ($SkipSign) { $args["SkipSign"] = $true }
        & (Join-Path $root "packaging\windows\build-installer.ps1") @args
    }
}
finally {
    Pop-Location
}
