# Installs mdv.exe to $(go env GOPATH)\bin and registers it with Explorer's
# "Open with" for .md / .markdown files. Per-user (HKCU): no admin needed,
# and it never takes over the default handler -- pick mdv once from
# "Open with > Choose another app" to make it the default.
#
#   powershell -ExecutionPolicy Bypass -File build\windows\install.ps1
#   powershell -ExecutionPolicy Bypass -File build\windows\install.ps1 -Uninstall
param([switch]$Uninstall)
$ErrorActionPreference = 'Stop'

$progId  = 'mdv.Markdown'
$classes = 'HKCU:\Software\Classes'
$exts    = '.md', '.markdown'

if ($Uninstall) {
    Remove-Item "$classes\$progId" -Recurse -ErrorAction SilentlyContinue
    Remove-Item "$classes\Applications\mdv.exe" -Recurse -ErrorAction SilentlyContinue
    foreach ($e in $exts) {
        Remove-ItemProperty "$classes\$e\OpenWithProgids" -Name $progId -ErrorAction SilentlyContinue
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

# Tell Explorer the associations changed (SHCNE_ASSOCCHANGED).
Add-Type -Namespace Win32 -Name Shell -MemberDefinition '[DllImport("shell32.dll")] public static extern void SHChangeNotify(int e, uint f, System.IntPtr a, System.IntPtr b);'
[Win32.Shell]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)

Write-Host "installed $exe and registered it under Open with for $($exts -join ', ')"
Write-Host "make sure $gobin is on PATH to run mdv from a shell"
