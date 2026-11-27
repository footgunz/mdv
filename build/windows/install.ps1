# Installs mdv.exe to $(go env GOPATH)\bin and registers it with Explorer's
# "Open with" for .md / .markdown files and in Settings > Default apps.
# Per-user (HKCU): no admin needed.
#
# -Default makes mdv the .md/.markdown handler. Windows 10/11 protect the
# user's choice (UserChoice is hash-signed), so a program can't set it
# silently: if a choice already exists, this opens mdv's page in Default apps
# for a one-click confirm; otherwise the per-user class default is enough.
#
#   powershell -ExecutionPolicy Bypass -File build\windows\install.ps1 [-Default]
#   powershell -ExecutionPolicy Bypass -File build\windows\install.ps1 -Uninstall
param([switch]$Uninstall, [switch]$Default)
$ErrorActionPreference = 'Stop'

$progId  = 'mdv.Markdown'
$classes = 'HKCU:\Software\Classes'
$exts    = '.md', '.markdown'

if ($Uninstall) {
    Remove-Item "$classes\$progId" -Recurse -ErrorAction SilentlyContinue
    Remove-Item "$classes\Applications\mdv.exe" -Recurse -ErrorAction SilentlyContinue
    Remove-Item 'HKCU:\Software\mdv' -Recurse -ErrorAction SilentlyContinue
    Remove-ItemProperty 'HKCU:\Software\RegisteredApplications' -Name mdv -ErrorAction SilentlyContinue
    foreach ($e in $exts) {
        Remove-ItemProperty "$classes\$e\OpenWithProgids" -Name $progId -ErrorAction SilentlyContinue
        # only undo a class default we set ourselves
        if ((Get-ItemProperty "$classes\$e" -ErrorAction SilentlyContinue).'(default)' -eq $progId) {
            Set-ItemProperty "$classes\$e" -Name '(default)' -Value ''
        }
    }
    Write-Host 'removed mdv file associations (the binary in GOPATH\bin is left in place)'
    return
}

$repo = Resolve-Path "$PSScriptRoot\..\.."
Push-Location $repo
try {
    $env:CGO_ENABLED = '0'
    go install -tags desktop,production ./cmd/mdv
    if ($LASTEXITCODE -ne 0) { throw 'go install failed' }
} finally { Pop-Location }

$gobin = (go env GOBIN)
if (-not $gobin) { $gobin = Join-Path (go env GOPATH) 'bin' }
$exe = Join-Path $gobin 'mdv.exe'
$cmd = "`"$exe`" `"%1`""

function Set-Default($path, $value) {
    New-Item $path -Force | Out-Null
    Set-ItemProperty $path -Name '(default)' -Value $value
}

Set-Default "$classes\$progId" 'Markdown document'
Set-Default "$classes\$progId\DefaultIcon" "`"$exe`",0"
Set-Default "$classes\$progId\shell\open\command" $cmd

# Applications\mdv.exe gives the "Open with" list a friendly name and icon.
New-Item "$classes\Applications\mdv.exe" -Force | Out-Null
Set-ItemProperty "$classes\Applications\mdv.exe" -Name FriendlyAppName -Value 'mdv'
Set-Default "$classes\Applications\mdv.exe\shell\open\command" $cmd
New-Item "$classes\Applications\mdv.exe\SupportedTypes" -Force | Out-Null

foreach ($e in $exts) {
    New-Item "$classes\$e\OpenWithProgids" -Force | Out-Null
    New-ItemProperty "$classes\$e\OpenWithProgids" -Name $progId -Value '' -PropertyType String -Force | Out-Null
    New-ItemProperty "$classes\Applications\mdv.exe\SupportedTypes" -Name $e -Value '' -PropertyType String -Force | Out-Null
}

# Capabilities + RegisteredApplications list mdv in Settings > Default apps.
$cap = 'HKCU:\Software\mdv\Capabilities'
New-Item "$cap\FileAssociations" -Force | Out-Null
Set-ItemProperty $cap -Name ApplicationName -Value 'mdv'
Set-ItemProperty $cap -Name ApplicationDescription -Value 'Markdown viewer with live reload and Mermaid'
foreach ($e in $exts) { Set-ItemProperty "$cap\FileAssociations" -Name $e -Value $progId }
New-Item 'HKCU:\Software\RegisteredApplications' -Force | Out-Null
Set-ItemProperty 'HKCU:\Software\RegisteredApplications' -Name mdv -Value 'Software\mdv\Capabilities'

$needsConfirm = $false
if ($Default) {
    foreach ($e in $exts) {
        # Honoured only while the user has made no explicit choice for $e.
        Set-ItemProperty "$classes\$e" -Name '(default)' -Value $progId
        $uc = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\$e\UserChoice"
        $cur = (Get-ItemProperty $uc -ErrorAction SilentlyContinue).ProgId
        if ($cur -and $cur -ne $progId) { $needsConfirm = $true }
    }
}

# Tell Explorer the associations changed (SHCNE_ASSOCCHANGED).
Add-Type -Namespace Win32 -Name Shell -MemberDefinition '[DllImport("shell32.dll")] public static extern void SHChangeNotify(int e, uint f, System.IntPtr a, System.IntPtr b);'
[Win32.Shell]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)

Write-Host "installed $exe and registered it under Open with for $($exts -join ', ')"
if ($needsConfirm) {
    Write-Host 'another app is your chosen .md handler; Windows requires you to confirm the switch.'
    Write-Host 'opening Settings > Default apps > mdv: set .md and .markdown to mdv there.'
    Start-Process 'ms-settings:defaultapps?registeredAppUser=mdv'
} elseif ($Default) {
    Write-Host 'mdv is now the default for .md and .markdown'
}

# CLI use from cmd/PowerShell needs GOPATH\bin on the user PATH.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $gobin)) {
    Write-Host "note: $gobin is not on your user PATH; add it to run mdv from cmd/PowerShell:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$gobin', 'User')"
}
