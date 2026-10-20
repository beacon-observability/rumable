#requires -Version 3.0
param([Parameter(Mandatory=$true)][string]$Report)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'windows-iis-helpers.ps1')
Assert-That ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT -and [Environment]::Is64BitProcess -and $PSVersionTable.PSEdition -ne 'Core') 'Use elevated 64-bit Windows PowerShell 3+, not PowerShell 7.'
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
Assert-That ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) 'Administrator privileges required.'
$record = [IO.File]::ReadAllText((Resolve-Path -LiteralPath $Report).Path) | ConvertFrom-Json
Assert-That ($record.schemaVersion -eq 1) 'Unsupported acceptance report.'
Add-Type -Path (Join-Path $env:SystemRoot 'System32/inetsrv/Microsoft.Web.Administration.dll')
Remove-TestDeployment $record.site $record.work
Write-Host ('Removed owned test site/pool/tree: ' + $record.site + '; original evidence report retained.')
