<#
.SYNOPSIS
    Installs the latest n8n CLI release for Windows.

.DESCRIPTION
    Downloads the n8n-windows-amd64.exe asset from the latest GitHub release
    into %LOCALAPPDATA%\n8n-cli\bin, and adds that directory to the user PATH
    when it is missing. Runs without administrator rights and never touches the
    machine PATH.

    Only the binary is written. Contexts and credentials are created by
    `n8n auth login`; nothing under the config directory is touched here.

    Environment overrides:
        N8N_CLI_VERSION      install this tag instead of the latest release
        N8N_CLI_INSTALL_DIR  install here instead of the default directory

.EXAMPLE
    irm https://raw.githubusercontent.com/SomeoneWithOptions/n8n-cli/main/install.ps1 | iex
#>

$ErrorActionPreference = 'Stop'

# Windows PowerShell 5.1 still defaults to SSL3/TLS1.0, which GitHub rejects.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = 'SomeoneWithOptions/n8n-cli'
$Binary = 'n8n.exe'

$InstallDir = $env:N8N_CLI_INSTALL_DIR
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path -Path $env:LOCALAPPDATA -ChildPath 'n8n-cli\bin'
}

$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -ne 'AMD64') {
    throw "Unsupported architecture: $Arch. Releases ship windows/amd64 only; download a binary manually from https://github.com/$Repo/releases"
}
$Asset = 'n8n-windows-amd64.exe'

$Latest = $env:N8N_CLI_VERSION
if ([string]::IsNullOrWhiteSpace($Latest)) {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Latest = $Release.tag_name
}
if ([string]::IsNullOrWhiteSpace($Latest)) {
    throw "Could not determine the latest release. Pin one with `$env:N8N_CLI_VERSION = 'vX.Y.Z'"
}

$Url = "https://github.com/$Repo/releases/download/$Latest/$Asset"
$Target = Join-Path -Path $InstallDir -ChildPath $Binary

Write-Output "Downloading n8n $Latest (windows/amd64)..."
if (-not (Test-Path -LiteralPath $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
Invoke-WebRequest -Uri $Url -OutFile $Target -UseBasicParsing

Write-Output "Installed to $Target"

$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$Entries = @()
if (-not [string]::IsNullOrWhiteSpace($UserPath)) {
    $Entries = $UserPath.Split(';') | Where-Object { $_ -ne '' }
}
if ($Entries -notcontains $InstallDir) {
    $Updated = (@($Entries) + $InstallDir) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $Updated, 'User')
    $env:Path = "$env:Path;$InstallDir"
    Write-Output "Added $InstallDir to your user PATH. Restart your terminal for it to apply everywhere."
}

Write-Output 'Next: n8n auth login --url https://your-n8n-instance'
