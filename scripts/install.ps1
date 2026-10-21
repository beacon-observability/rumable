param(
    [Parameter(Mandatory=$true)][string]$BaseUrl,
    [Parameter(Mandatory=$true)][string]$Config
)
$ErrorActionPreference = 'Stop'
$releaseVersion = '@@VERSION@@'
$digest = '@@WINDOWS_AMD64_SHA256@@'
if ($releaseVersion.Contains('@@') -or $digest.Contains('@@')) { throw 'Use the CI-published installer, not the source template' }
$source = [Uri]$BaseUrl
if (-not $source.IsAbsoluteUri -or $source.Scheme -ne 'https' -or $source.UserInfo -or $source.Query -or $source.Fragment) { throw 'BaseUrl must be a public HTTPS directory' }
if ($env:OS -ne 'Windows_NT' -or $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') { throw 'This installer requires Windows x64 and 64-bit PowerShell' }
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator' }
$bytes = [Convert]::FromBase64String($Config)
if ($bytes.Length -eq 0) { throw 'Empty configuration' }
$work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
[void][IO.Directory]::CreateDirectory($work)
try {
    [IO.File]::WriteAllBytes((Join-Path $work 'rumable.toml'), $bytes)
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $download = $BaseUrl.TrimEnd('/') + '/rumable_' + $releaseVersion + '_windows_amd64.zip'
    Invoke-WebRequest -UseBasicParsing -Uri $download -OutFile (Join-Path $work 'package.zip')
    if ((Get-FileHash (Join-Path $work 'package.zip') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $digest) { throw 'Package checksum mismatch' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $work 'package.zip'))
    try {
        $entry = $zip.GetEntry('rumable.exe')
        if ($null -eq $entry) { throw 'Package is missing rumable.exe' }
        [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $work 'rumable.exe'))
    } finally { $zip.Dispose() }
    $binDir = Join-Path $env:ProgramFiles ('Rumable\' + $releaseVersion)
    [void][IO.Directory]::CreateDirectory($binDir)
    $bin = Join-Path $binDir 'rumable.exe'
    if (Test-Path -LiteralPath $bin) {
        if ((Get-FileHash $bin).Hash -ne (Get-FileHash (Join-Path $work 'rumable.exe')).Hash) { throw 'Installed version has different bytes' }
    } else { Copy-Item -LiteralPath (Join-Path $work 'rumable.exe') -Destination $bin }
    & $bin setup --config (Join-Path $work 'rumable.toml') --state-dir (Join-Path $env:ProgramData 'Rumable') --yes
    if ($LASTEXITCODE -ne 0) { throw 'Setup failed; retain the state directory and inspect the reported cause' }
} finally { Remove-Item -LiteralPath $work -Recurse -Force }
