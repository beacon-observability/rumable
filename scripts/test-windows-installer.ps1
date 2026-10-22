# Portable installer contracts; Windows APIs/IIS still require native acceptance.
$ErrorActionPreference = 'Stop'
function Assert-That([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Assert-Rejected([scriptblock]$Action, [string]$Pattern) {
    try { & $Action } catch {
        Assert-That ($_.Exception.Message -match $Pattern) ('Unexpected error: ' + $_.Exception.Message)
        return
    }
    throw ('Expected rejection: ' + $Pattern)
}
$root = Join-Path ([IO.Path]::GetTempPath()) ('rumable-windows-installer-' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($root)
try {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $text = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'install.ps1')).Replace('@@VERSION@@','0.1.3')
    $script:packages = @{}
    foreach ($target in @('386','amd64','arm64','386_legacy','amd64_legacy')) {
        $name = 'rumable_0.1.3_windows_' + $target + '.zip'
        $archive = Join-Path $root $name
        $zip = [IO.Compression.ZipFile]::Open($archive, [IO.Compression.ZipArchiveMode]::Create)
        try {
            $entry = $zip.CreateEntry('rumable.exe')
            $stream = $entry.Open()
            try { $bytes = [Text.Encoding]::UTF8.GetBytes('executable fixture ' + $target); $stream.Write($bytes,0,$bytes.Length) } finally { $stream.Dispose() }
        } finally { $zip.Dispose() }
        $script:packages[$name] = $archive
        $text = $text.Replace(('@@WINDOWS_' + $target.ToUpperInvariant() + '_SHA256@@'), (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant())
    }
    $tokens=$null; $errors=$null
    $ast = [Management.Automation.Language.Parser]::ParseInput($text,[ref]$tokens,[ref]$errors)
    Assert-That ($errors.Count -eq 0) ($errors | Out-String)
    # Load only the real function definitions; never run the top-level installer.
    foreach ($statement in $ast.EndBlock.Statements) {
        if ($statement -is [Management.Automation.Language.FunctionDefinitionAst]) { Invoke-Expression $statement.Extent.Text }
    }
    $discoveryScript = Join-Path (Split-Path $PSScriptRoot) 'internal/rumable/assets/iis-discover.ps1'
    [void][Management.Automation.Language.Parser]::ParseFile($discoveryScript,[ref]$tokens,[ref]$errors)
    Assert-That ($errors.Count -eq 0) ($errors | Out-String)
    function Test-RumableBinary { $script:cliArgs = @($args); $global:LASTEXITCODE = 0 }
    function Test-RumableNonInteractive { return $false }
    Invoke-RumableSetup 'Test-RumableBinary' 'config with spaces.toml' 'state with spaces' -Discover
    Assert-That (($script:cliArgs -join '|') -eq 'setup|--config|config with spaces.toml|--state-dir|state with spaces|--yes|--discover') 'CLI discovery arguments changed or split paths'
    Invoke-RumableSetup 'Test-RumableBinary' 'config.toml' 'state'
    Assert-That (-not ($script:cliArgs -contains '--discover')) 'Manual setup unexpectedly enabled discovery'
    Invoke-RumableSetup 'Test-RumableBinary' 'config.toml' 'state' -Discover -NonInteractive
    Assert-That ($script:cliArgs -contains '--non-interactive') 'Explicit noninteractive mode was lost'
    function Test-RumableNonInteractive { return $true }
    Invoke-RumableSetup 'Test-RumableBinary' 'config.toml' 'state' -Discover
    Assert-That ($script:cliArgs -contains '--non-interactive') 'Redirected input still prompts'
    Initialize-RumableNativeSystem # Compile the native API bridge without calling Windows DLLs.
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        Assert-Rejected { Get-RumableWindowsHost } 'requires Windows'
    }
    foreach ($case in @(
        @('amd64','10.0.19045',0,'modern'), @('arm64','10.0.22631',0,'modern'), @('386','10.0.19045',0,'modern'),
        @('amd64','6.1.7601',1,'legacy'), @('386','6.1.7601',1,'legacy'),
        @('amd64','6.2.9200',0,'legacy'), @('amd64','6.3.9600',0,'legacy')
    )) {
        $p = Get-RumableWindowsPackage $case[0] ([Version]$case[1]) $case[2] '0.1.3'
        Assert-That ($p.Architecture -eq $case[0] -and $p.Flavor -eq $case[3]) ('Wrong package for ' + ($case -join '/'))
        Assert-That ($p.Digest -eq (Get-FileHash $packages[$p.File] -Algorithm SHA256).Hash.ToLowerInvariant()) 'Wrong pinned package checksum'
    }
    Assert-Rejected { Get-RumableWindowsPackage 'amd64' ([Version]'6.1.7600') 0 '0.1.3' } 'SP1'
    Assert-Rejected { Get-RumableWindowsPackage 'amd64' ([Version]'6.0.6002') 2 '0.1.3' } 'SP1'
    Assert-Rejected { Get-RumableWindowsPackage 'arm64' ([Version]'6.3.9600') 0 '0.1.3' } 'ARM64 requires'
    Assert-Rejected { Get-RumableWindowsPackage 'ia64' ([Version]'10.0.1') 0 '0.1.3' } 'Supported Windows architectures'
    Assert-That ((Get-RumableProgramFiles $true $false 'x86-programs' 'native-programs') -eq 'native-programs') 'WOW64 used the 32-bit Program Files directory'
    Assert-That ((Get-RumableProgramFiles $true $true 'native-programs' '') -eq 'native-programs') 'Native Program Files changed'
    Assert-Rejected { Get-RumableProgramFiles $true $false 'x86-programs' '' } 'Cannot locate'
    # Only OS discovery, network and CLI execution are mocked. Real package
    # selection, hashing, ZIP extraction, file installation and cleanup run below.
    function Get-RumableWindowsHost { return $script:hostInfo }
    function Invoke-WebRequest([switch]$UseBasicParsing, [string]$Uri, [string]$OutFile) {
        $script:downloads += $Uri
        $script:lastWork = Split-Path $OutFile
        $file = ([Uri]$Uri).Segments[-1]
        if ($script:corrupt) { [IO.File]::WriteAllText($OutFile,'corrupt'); return }
        Copy-Item -LiteralPath $script:packages[$file] -Destination $OutFile
    }
    function Invoke-RumableSetup([string]$Binary, [string]$ConfigPath, [string]$StateDirectory, [switch]$Discover, [switch]$NonInteractive) {
        $script:setups++
        Assert-That ($Discover -eq $script:expectDiscovery) 'Discovery flag was not forwarded to CLI'
        Assert-That ([IO.File]::ReadAllText($ConfigPath) -ceq $script:configText) 'Configuration was changed or executed'
        Assert-That ($StateDirectory -eq (Join-Path $script:hostInfo.ProgramData 'Rumable')) 'Wrong state directory'
        Assert-That ($Binary -eq (Join-Path (Join-Path (Join-Path $script:hostInfo.ProgramFiles 'Rumable') '0.1.3') 'rumable.exe')) 'Wrong binary directory'
        if ($script:failSetup) { throw 'setup fixture failed' }
    }
    $script:configText = "schemaVersion = 1`n# " + [char]0x4e2d + " ' ; `$() <test>`n"
    $config = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($script:configText))
    foreach ($case in @(@('amd64','10.0.19045',0), @('amd64','6.1.7601',1), @('arm64','10.0.22631',0))) {
        $script:hostInfo = @{Architecture=$case[0];Version=[Version]$case[1];ServicePack=$case[2];ProgramFiles=(Join-Path $root ($case -join '-'));ProgramData=(Join-Path $root 'data')}
        $script:downloads=@(); $script:setups=0; $script:expectDiscovery=$false; $script:corrupt=$false; $script:failSetup=$false
        Invoke-RumableWindowsInstall 'https://downloads.example.com/rumable/latest/' $config
        $script:expectDiscovery=$true
        Invoke-RumableWindowsInstall 'https://downloads.example.com/rumable/latest' $config -Discover
        $script:expectDiscovery=$false
        $p = Get-RumableWindowsPackage $case[0] ([Version]$case[1]) $case[2] '0.1.3'
        Assert-That ($script:downloads.Count -eq 2 -and $script:setups -eq 2) 'First/repeated installation did not reach setup'
        Assert-That ($script:downloads[0] -eq ('https://downloads.example.com/rumable/0.1.3/' + $p.File)) 'latest downloaded a wrong or mutable package'
        Assert-That (-not (Test-Path $script:lastWork)) 'Successful install leaked temporary configuration'
        $script:corrupt=$true
        Assert-Rejected { Invoke-RumableWindowsInstall 'https://downloads.example.com/rumable/0.1.3' $config } 'checksum mismatch'
        Assert-That ($script:setups -eq 2 -and -not (Test-Path $script:lastWork)) 'Corrupt archive reached setup or leaked temporary files'
        $script:corrupt=$false; $script:failSetup=$true
        Assert-Rejected { Invoke-RumableWindowsInstall 'https://downloads.example.com/rumable/0.1.3' $config } 'setup fixture failed'
        Assert-That (-not (Test-Path $script:lastWork)) 'Failed setup leaked temporary configuration'
    }
    $script:downloads=@()
    Assert-Rejected { Invoke-RumableWindowsInstall 'http://example.com' $config } 'HTTPS'
    Assert-Rejected { Invoke-RumableWindowsInstall 'https://downloads.example.com' '!!' } 'Base-64'
    $script:hostInfo.Architecture='386'
    Assert-Rejected { Invoke-RumableWindowsInstall 'https://downloads.example.com' $config } '32-bit Windows'
    $script:hostInfo.Architecture='amd64'; $script:hostInfo.Version=[Version]'6.1.7600'; $script:hostInfo.ServicePack=0
    Assert-Rejected { Invoke-RumableWindowsInstall 'https://downloads.example.com' $config } 'SP1'
    Assert-That ($script:downloads.Count -eq 0) 'Unsupported input reached download'
    Write-Host 'PASS: Windows package matrix, WOW64 install directory, native bridge compilation, pinned download, real ZIP/hash/install, repeat install, rejection and cleanup. Windows/IIS APIs were not executed.'
} finally { Remove-Item -LiteralPath $root -Recurse -Force }
