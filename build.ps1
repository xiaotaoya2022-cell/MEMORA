param(
    [string]$Output = "bin\memora.exe"
)

$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "0"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location $root
try {
    go build -trimpath -ldflags "-s -w" -o $Output ./cmd/memora
    $item = Get-Item -LiteralPath (Join-Path $root $Output)
    "built: {0} ({1:N0} bytes)" -f $item.FullName, $item.Length
}
finally {
    Pop-Location
}
