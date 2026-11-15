param(
    [string[]]$Files = @(),
    [switch]$EnsureCertOnly,
    [string]$CertOutputPath = ""
)

$ErrorActionPreference = "Stop"

$pkgDir  = $PSScriptRoot
$rootDir = Split-Path (Split-Path $pkgDir -Parent) -Parent
$relDir  = Join-Path $rootDir "bin\release"
New-Item -ItemType Directory -Force -Path $relDir | Out-Null

$defaultCerPkg = Join-Path $pkgDir "MEMORA-CodeSign.cer"
$defaultCerRel = Join-Path $relDir "MEMORA-CodeSign.cer"
$trustCmdRel   = Join-Path $relDir "install-trust-cert.cmd"

function Find-SignTool {
    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    $candidates = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue |
        Sort-Object FullName -Descending
    if ($candidates -and $candidates.Count -gt 0) {
        return $candidates[0].FullName
    }
    $x86Candidates = Get-ChildItem -Path "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x86\signtool.exe" -ErrorAction SilentlyContinue |
        Sort-Object FullName -Descending
    if ($x86Candidates -and $x86Candidates.Count -gt 0) {
        return $x86Candidates[0].FullName
    }
    throw "signtool.exe not found. Please install Windows 10/11 SDK Signing Tools."
}

function Import-CertToStore {
    param(
        [System.Security.Cryptography.X509Certificates.X509Certificate2]$Cert,
        [System.Security.Cryptography.X509Certificates.StoreName]$StoreName,
        [System.Security.Cryptography.X509Certificates.StoreLocation]$Location
    )
    $store = New-Object System.Security.Cryptography.X509Certificates.X509Store($StoreName, $Location)
    try {
        $store.Open([System.Security.Cryptography.X509Certificates.OpenFlags]::ReadWrite)
        $existing = $store.Certificates.Find([System.Security.Cryptography.X509Certificates.X509FindType]::FindByThumbprint, $Cert.Thumbprint, $false)
        if ($existing.Count -eq 0) {
            $pubBytes = $Cert.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
            $pubCert  = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2(,$pubBytes)
            $store.Add($pubCert)
        }
    } finally {
        $store.Close()
    }
}

$cert = $null
$usedPfx = $false
$pfxPath = $env:MEMORA_SIGN_PFX
$pfxPass = $env:MEMORA_SIGN_PFX_PASSWORD
$thumb   = $env:MEMORA_SIGN_THUMBPRINT

if ($pfxPath -and (Test-Path $pfxPath)) {
    $usedPfx = $true
    if ($pfxPass) {
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($pfxPath, $pfxPass)
    } else {
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($pfxPath)
    }
} else {
    if ($thumb) {
        foreach ($loc in @("Cert:\CurrentUser\My", "Cert:\LocalMachine\My")) {
            $found = Get-ChildItem -Path $loc -ErrorAction SilentlyContinue |
                Where-Object { $_.Thumbprint -ieq $thumb.Trim() } | Select-Object -First 1
            if ($found) { $cert = $found; break }
        }
        if (-not $cert) {
            throw "MEMORA_SIGN_THUMBPRINT '$thumb' not found in CurrentUser\My or LocalMachine\My."
        }
    } else {
        $minExpiry = (Get-Date).AddDays(30)
        $cert = Get-ChildItem -Path "Cert:\CurrentUser\My" -CodeSigningCert -ErrorAction SilentlyContinue |
            Where-Object { $_.Subject -like "*CN=MEMORA Project*" -and $_.HasPrivateKey -and $_.NotAfter -gt $minExpiry } |
            Sort-Object NotAfter -Descending |
            Select-Object -First 1

        if (-not $cert) {
            Write-Host "[SIGN] Creating persistent MEMORA Code Signing certificate in Cert:\CurrentUser\My..."
            $cert = New-SelfSignedCertificate `
                -Type CodeSigningCert `
                -Subject "CN=MEMORA Project, O=MEMORA Open Source, C=CN" `
                -FriendlyName "MEMORA Official Code Signing" `
                -KeyUsage DigitalSignature `
                -KeyAlgorithm RSA `
                -KeyLength 2048 `
                -HashAlgorithm SHA256 `
                -CertStoreLocation "Cert:\CurrentUser\My" `
                -NotAfter (Get-Date).AddYears(5)
        }
    }
}

# Export public .cer file
$cerBytes = $cert.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Cert)
[System.IO.File]::WriteAllBytes($defaultCerPkg, $cerBytes)
[System.IO.File]::WriteAllBytes($defaultCerRel, $cerBytes)
if ($CertOutputPath -and ($CertOutputPath -ne $defaultCerPkg) -and ($CertOutputPath -ne $defaultCerRel)) {
    [System.IO.File]::WriteAllBytes($CertOutputPath, $cerBytes)
}

# Write helper script for portable/pre-install trust setup (self-elevates to register LocalMachine Root + TrustedPublisher)
$trustCmdContent = @'
@echo off
setlocal
cd /d "%~dp0"
if not exist "MEMORA-CodeSign.cer" (
    echo [ERROR] MEMORA-CodeSign.cer not found next to this script.
    pause
    exit /b 1
)
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [MEMORA] Requesting Administrator elevation to install trusted code signing certificate...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b %errorlevel%
)
echo [MEMORA] Registering MEMORA-CodeSign.cer into LocalMachine\Root and LocalMachine\TrustedPublisher...
certutil.exe -addstore -f "Root" "%~dp0MEMORA-CodeSign.cer"
if %errorlevel% neq 0 (
    echo [ERROR] Failed to import certificate into LocalMachine\Root.
    pause
    exit /b %errorlevel%
)
certutil.exe -addstore -f "TrustedPublisher" "%~dp0MEMORA-CodeSign.cer"
if %errorlevel% neq 0 (
    echo [ERROR] Failed to import certificate into LocalMachine\TrustedPublisher.
    pause
    exit /b %errorlevel%
)
echo [MEMORA] Trust chain installed successfully. Windows now verifies MEMORA binaries as Valid.
timeout /t 3 >nul
exit /b 0
'@
[System.IO.File]::WriteAllText($trustCmdRel, $trustCmdContent, [System.Text.Encoding]::ASCII)

# Ensure self-signed (or local) certificate is trusted cleanly without triggering blocking non-admin Root store UI dialogs
if (-not $usedPfx -and $cert.Issuer -eq $cert.Subject) {
    Import-CertToStore -Cert $cert -StoreName TrustedPublisher -Location CurrentUser

    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if ($isAdmin) {
        & certutil.exe -addstore -f "Root" $defaultCerPkg | Out-Null
        & certutil.exe -addstore -f "TrustedPublisher" $defaultCerPkg | Out-Null
    } else {
        Write-Host "[SIGN] Note: current shell is non-elevated; skipping LocalMachine\Root import to avoid interactive Security Warning popup."
        Write-Host "[SIGN] Run install-trust-cert.cmd (elevated) or the signed setup installer to register MEMORA-CodeSign.cer in LocalMachine\Root."
    }
}

if ($EnsureCertOnly -or $Files.Count -eq 0) {
    Write-Host "[SIGN] Certificate ready: $($cert.Thumbprint) ($($cert.Subject))"
    exit 0
}

$signtool = Find-SignTool
$timestampUrl = $env:MEMORA_TIMESTAMP_URL

foreach ($file in $Files) {
    if (-not (Test-Path $file)) {
        throw "Target file for signing not found: $file"
    }
    $fullFile = (Resolve-Path $file).Path

    $signArgs = @("sign", "/fd", "SHA256")
    if ($usedPfx) {
        $signArgs += @("/f", $pfxPath)
        if ($pfxPass) {
            $signArgs += @("/p", $pfxPass)
        }
    } else {
        $signArgs += @("/sha1", $cert.Thumbprint)
    }
    if ($timestampUrl) {
        $signArgs += @("/tr", $timestampUrl, "/td", "SHA256")
    }
    $signArgs += @("/d", "MEMORA", "/du", "https://github.com/memora-project/memora", $fullFile)

    & $signtool @signArgs
    if ($LASTEXITCODE -ne 0) {
        throw "signtool.exe failed for $fullFile (exit code $LASTEXITCODE)"
    }

    Unblock-File -Path $fullFile -ErrorAction SilentlyContinue

    $sig = Get-AuthenticodeSignature -FilePath $fullFile
    if (-not $sig -or $sig.Status -eq "NotSigned" -or $sig.Status -eq "HashMismatch") {
        throw "Signature verification failed on $fullFile : Status=$($sig.Status) ($($sig.StatusMessage))"
    }
    Write-Host "[SIGN] Signed $fullFile -> Status=$($sig.Status) Thumbprint=$($sig.SignerCertificate.Thumbprint)"
}
