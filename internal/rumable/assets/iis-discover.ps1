# Read-only inventory. Never commit IIS configuration during discovery.
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
if (-not [Environment]::Is64BitProcess) { throw 'IIS discovery requires a 64-bit PowerShell host.' }
Add-Type -Path (Join-Path $env:SystemRoot 'System32\inetsrv\Microsoft.Web.Administration.dll')
$manager = New-Object Microsoft.Web.Administration.ServerManager
try {
    $sites = @()
    foreach ($site in $manager.Sites) {
        if ($site.State -ne 'Started') { continue }
        $bindings = @($site.Bindings | Where-Object { $_.Protocol -in @('http','https') } | ForEach-Object {
            @{Protocol=$_.Protocol;Host=$_.Host;Address=$_.EndPoint.Address.ToString();Port=$_.EndPoint.Port}
        })
        if ($bindings.Count -eq 0) { continue }
        foreach ($application in $site.Applications) {
            $target = $site.Name
            if ($application.Path -ne '/') { $target += $application.Path }
            $compression = $manager.GetApplicationHostConfiguration().GetSection('system.webServer/urlCompression', $target)
            $sites += @{Name=$site.Name;Application=$application.Path;Bindings=$bindings;StaticCompression=[bool]$compression.GetAttributeValue('doStaticCompression')}
        }
    }
    @{Sites=@($sites)} | ConvertTo-Json -Depth 8 -Compress
} finally { $manager.Dispose() }
