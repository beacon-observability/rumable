#requires -Version 3.0
param(
    [Parameter(Mandatory=$true)][string]$Binary,
    [string]$PreviousBinary,
    [ValidateRange(1024,65535)][int]$Port = 19182,
    [ValidateSet('7.5','8.0','8.5','10.0')][string]$ExpectedIISVersion,
    [string]$ResultDir,
    [switch]$StaticOnly,
    [switch]$Keep
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'windows-iis-helpers.ps1')
$runId = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$name = 'Rumable-Acceptance-' + $runId
$work = Join-Path $env:TEMP $name
$web = Join-Path $work 'www'
$stateDir = Join-Path $work 'state'
if (-not $ResultDir) { $ResultDir = Join-Path (Get-Location).Path ('rumable-iis-results/' + $runId) }
$ResultDir = [IO.Path]::GetFullPath($ResultDir)
Assert-That (-not (Test-Path -LiteralPath $ResultDir)) 'ResultDir must be a new directory.'
[void][IO.Directory]::CreateDirectory($ResultDir)
$log = Join-Path $ResultDir 'cli.log'
Write-Utf8 $log ''
$report = [ordered]@{
    schemaVersion = 1; startedAt = [DateTime]::UtcNow.ToString('o'); result = 'running'
    environment = @{}; steps = @(); cleanup = @{}; site = $name; work = $work
    boundaries = @('HTTP injection/configuration only; browser and Guance ingestion unverified', 'No existing site, global module, section delegation or service restart is modified')
}
$created = $false
$parentValue = 'parent'
$baseUrl = 'http://127.0.0.1:' + $Port

function Invoke-Step([string]$StepName, [scriptblock]$Action) {
    Write-Host ('[RUN] ' + $StepName)
    $timer = [Diagnostics.Stopwatch]::StartNew()
    try {
        & $Action
        $report.steps += @{ name = $StepName; result = 'passed'; elapsedMs = $timer.ElapsedMilliseconds }
        Write-Host ('[PASS] ' + $StepName)
    } catch {
        $report.steps += @{ name = $StepName; result = 'failed'; elapsedMs = $timer.ElapsedMilliseconds; error = $_.Exception.Message }
        throw
    } finally {
        $timer.Stop()
        Write-Utf8 (Join-Path $ResultDir 'report.json') ($report | ConvertTo-Json -Depth 60)
    }
}

function Invoke-Cli([string]$Command, [switch]$Reject, [string]$Exe = $Binary) {
    return (Invoke-TestCli $Exe @($Command, '--config', $configPath, '--site', $scenario.id, '--state-dir', $stateDir) $log -ExpectFailure:$Reject)
}

function Assert-DeploymentUnchanged([string]$BeforeKey) {
    Assert-That ((Get-DeploymentKey (Read-TestState $stateDir $scenario.id)) -ceq $BeforeKey) 'Rejected operation changed deployment/history.'
    Assert-That (-not (Test-Path -LiteralPath (Join-Path $stateDir ('sites/' + $scenario.id + '/transaction.json')))) 'Recovered operation left an unfinished journal.'
}

function Read-BinaryInfo([string]$Path) {
	$build = $null
    $metadata = Join-Path ([IO.Path]::GetDirectoryName($Path)) 'build.json'
    if (Test-Path -LiteralPath $metadata) {
        $build = [IO.File]::ReadAllText($metadata) | ConvertFrom-Json
        Assert-That ($build.os -eq 'windows' -and @('386','amd64','arm64') -contains $build.arch) 'Select a Windows x86, x64 or ARM64 release matching the host.'
        if ($script:osVersion.Major -lt 10) { Assert-That ($build.go -eq 'go1.20.14') 'Old Windows requires the legacy Go 1.20.14 release; binary was not launched.' }
    }
    $result = Invoke-TestCli $Path @('version') $log
    try { $version = $result.output | ConvertFrom-Json }
    catch { throw 'Binary did not return version JSON. Check cli.log; old Windows must use the legacy package.' }
    Assert-That ($version.host.os -eq 'windows' -and @('386','amd64','arm64') -contains $version.host.arch) 'Binary did not identify as a supported Windows executable.'
    if ($null -ne $build) { Assert-That ($version.host.arch -eq $build.arch -and $version.version -eq $build.version) 'Executable architecture/version differs from build.json.' }
    if ($script:osVersion.Major -lt 10) { Assert-That ($version.host.go -eq 'go1.20.14') 'Old Windows acceptance requires a real Go 1.20.14 build.' }
    return @{ path = $Path; sha256 = Get-SHA256 ([IO.File]::ReadAllBytes($Path)); version = $version }
}

try {
    Invoke-Step 'environment-and-binary' {
        Assert-That ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) 'Run this script on Windows.'
        Assert-That ([Environment]::Is64BitProcess -and $PSVersionTable.PSEdition -ne 'Core') 'Use 64-bit Windows PowerShell 3+ (5.1 recommended), not PowerShell 7.'
        $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
        Assert-That ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) 'Run an elevated Administrator PowerShell.'
        $os = Get-WmiObject Win32_OperatingSystem
        $script:osVersion = [version]$os.Version
        Assert-That ($osVersion -ge [version]'6.1') 'Windows 7 SP1 / Server 2008 R2 SP1 or newer is required.'
        if ($osVersion.Major -eq 6 -and $osVersion.Minor -eq 1) { Assert-That ($os.ServicePackMajorVersion -ge 1) 'Windows 7 / Server 2008 R2 requires SP1.' }
        $iis = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\InetStp'
        $iisVersion = ([string]$iis.MajorVersion) + '.' + $iis.MinorVersion
        Assert-That ([version]$iisVersion -ge [version]'7.5') 'IIS 7.5+ required.'
        if ($ExpectedIISVersion) { Assert-That ($iisVersion -eq $ExpectedIISVersion) ('Expected IIS ' + $ExpectedIISVersion + ', found ' + $iisVersion) }
        Add-Type -Path (Join-Path $env:SystemRoot 'System32/inetsrv/Microsoft.Web.Administration.dll')
        $manager = New-Object Microsoft.Web.Administration.ServerManager
        try {
            $config = $manager.GetApplicationHostConfiguration()
            [void]$config.GetSection('system.webServer/rewrite/outboundRules').GetCollection()
            $modules = @($config.GetSection('system.webServer/globalModules').GetCollection() | ForEach-Object { $_.GetAttributeValue('name') })
            Assert-That ($modules -contains 'StaticCompressionModule') 'Install IIS Static Content Compression before full acceptance.'
            if (-not $StaticOnly) {
                $handlers = @($config.GetSection('system.webServer/handlers').GetCollection() | ForEach-Object { $_.GetAttributeValue('name') })
                Assert-That (($modules -contains 'ManagedEngineV4.0_64bit') -and ($handlers -contains 'PageHandlerFactory-Integrated-4.0')) 'Enable/register 64-bit ASP.NET 4.x for the full suite, or use StaticOnly for partial acceptance.'
            }
            $rewriteModule = @($config.GetSection('system.webServer/globalModules').GetCollection() | Where-Object { $_.GetAttributeValue('name') -eq 'RewriteModule' }) | Select-Object -First 1
            Assert-That ($null -ne $rewriteModule) 'URL Rewrite 2.x must be installed.'
            $rewritePath = [Environment]::ExpandEnvironmentVariables([string]$rewriteModule.GetAttributeValue('image'))
            $rewriteVersion = [Diagnostics.FileVersionInfo]::GetVersionInfo($rewritePath).FileVersion
            Assert-That ($null -eq $manager.Sites[$name] -and $null -eq $manager.ApplicationPools[$name]) 'Test resource name collision.'
            foreach ($site in $manager.Sites) {
                foreach ($binding in $site.Bindings) {
                    if ($binding.Protocol -eq 'http' -and ($binding.BindingInformation -split ':')[-2] -eq [string]$Port) { throw 'Port is already used by an IIS binding.' }
                }
            }
        } finally { $manager.Dispose() }
        $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $Port)
        try { $listener.Start() } finally { $listener.Stop() }
        $script:Binary = (Resolve-Path -LiteralPath $Binary).Path
        if ($PreviousBinary) { $script:PreviousBinary = (Resolve-Path -LiteralPath $PreviousBinary).Path }
        $report.environment = @{
            os = $os.Caption; osVersion = $os.Version; servicePack = $os.ServicePackMajorVersion; architecture = $os.OSArchitecture
            powershell = $PSVersionTable.PSVersion.ToString(); clr = [Environment]::Version.ToString(); iis = $iisVersion
            rewrite = $rewriteVersion; binary = Read-BinaryInfo $Binary
        }
        if ($PreviousBinary) { $report.environment.previousBinary = Read-BinaryInfo $PreviousBinary }
    }

    $scenarios = @(
        @{ id='inherited'; app='/Inherited'; present=$false; value=$null; effective=$true },
        @{ id='local-on'; app='/LocalOn'; present=$true; value='true'; effective=$true },
        @{ id='local-off'; app='/LocalOff'; present=$true; value='false'; effective=$false }
    )
    Invoke-Step 'create-isolated-site-and-inheritance' {
        Assert-That (-not (Test-Path -LiteralPath $work)) 'Temporary test directory already exists.'
        [void][IO.Directory]::CreateDirectory($web)
        [void][IO.Directory]::CreateDirectory($stateDir)
        $html = '<!doctype html><html><head><title>IIS acceptance</title></head><body>' + ('fixture text ' * 4096) + '</body></html>'
        Write-Utf8 (Join-Path $web 'index.html') $html
        foreach ($p in $scenarios) {
            $folder = Join-Path $web $p.id
            [void][IO.Directory]::CreateDirectory((Join-Path $folder 'data'))
            Write-Utf8 (Join-Path $folder 'index.html') $html
            Write-Utf8 (Join-Path $folder 'excluded/index.html') $html
            Write-Utf8 (Join-Path $folder 'nohead.html') '<html><body>No head</body></html>'
            Write-Utf8 (Join-Path $folder 'data.json') '{"value":"</head>"}'
            Write-Utf8 (Join-Path $folder 'data/fail-counter.txt') '0'
            if (-not $StaticOnly) {
                foreach ($leaf in @('dynamic','chunked','gzip','fail','nohead','data','download')) {
                    [IO.File]::Copy((Join-Path $PSScriptRoot 'windows-fixture.aspx'), (Join-Path $folder ($leaf + '.aspx')))
                }
            }
        }
        $manager = New-Object Microsoft.Web.Administration.ServerManager
        try {
            $pool = $manager.ApplicationPools.Add($name)
            $pool.ManagedRuntimeVersion = ''; if (-not $StaticOnly) { $pool.ManagedRuntimeVersion = 'v4.0' }
            $pool.ManagedPipelineMode = 'Integrated'
            $site = $manager.Sites.Add($name, 'http', ('127.0.0.1:' + $Port + ':'), $web)
            $site.Applications['/'].ApplicationPoolName = $name
            $config = $manager.GetApplicationHostConfiguration()
            $compression = $config.GetSection('system.webServer/urlCompression', $name)
            $compression.SetAttributeValue('doStaticCompression', $true)
            $compression.SetAttributeValue('doDynamicCompression', $false)
            $config.GetSection('system.webServer/rewrite/outboundRules', $name).SetAttributeValue('rewriteBeforeCache', $false)
            Add-TestHeaderRule $config $name 'acceptance-parent' 'RESPONSE_X_Rumable_Parent' $parentValue
            foreach ($p in $scenarios) {
                $app = $site.Applications.Add($p.app, (Join-Path $web $p.id))
                $app.ApplicationPoolName = $name
                Add-TestHeaderRule $config ($name + $p.app) 'acceptance-child' 'RESPONSE_X_Rumable_Child' 'child'
                if ($p.present) { $config.GetSection('system.webServer/urlCompression', ($name + $p.app)).SetAttributeValue('doStaticCompression', [bool]::Parse($p.value)) }
            }
            $manager.CommitChanges()
            $script:created = $true
            $report.siteId = $site.Id
        } finally { $manager.Dispose() }
        & icacls.exe $work /grant '*S-1-5-32-568:(OI)(CI)RX' /T /Q | Out-Null
        Assert-That ($LASTEXITCODE -eq 0) 'Failed to grant test-only IIS read access.'
        if (-not $StaticOnly) {
            foreach ($p in $scenarios) {
                & icacls.exe (Join-Path $web ($p.id + '/data')) /grant '*S-1-5-32-568:(OI)(CI)M' /T /Q | Out-Null
                Assert-That ($LASTEXITCODE -eq 0) 'Failed to grant test-only failure-counter write access.'
            }
        }
        $ready = $false
        for ($i=0; $i -lt 20; $i++) {
            try { $page = Invoke-TestHttp ($baseUrl + '/Inherited/index.html'); $ready = $page.status -eq 200 } catch {}
            if ($ready) { break }; Start-Sleep -Milliseconds 500
        }
        if ($null -ne $page) { $report.startupHttpStatus = $page.status }
        Assert-That $ready 'Disposable site did not start. Check roles, ACL, inherited locks and anonymous/static handlers.'
    }

    foreach ($scenario in $scenarios) {
        $configPath = Join-Path $work ($scenario.id + '.toml')
        $paths = @($scenario.app + '/index.html')
        if (-not $StaticOnly) { $paths += ($scenario.app + '/dynamic.aspx'); $paths += ($scenario.app + '/chunked.aspx') }
        $normalConfig = New-TestConfig $name $scenario.id $scenario.app $Port $paths
        $business = Get-TestBusinessRules $name
        Invoke-Step ($scenario.id + '/baseline-and-render') {
            $effective = Get-TestEffective $name $scenario.app
            Assert-That ($effective.static -eq $scenario.effective -and -not $effective.dynamic) 'Compression baseline mismatch.'
            Assert-TestStaticGzip ($baseUrl + $scenario.app + '/index.html') $scenario.effective
            Write-Utf8 $configPath $normalConfig
            $initialBinary = $Binary; if ($PreviousBinary) { $initialBinary = $PreviousBinary }
            [void](Invoke-Cli 'render' -Exe $initialBinary)
        }
        Invoke-Step ($scenario.id + '/compression-consent-and-inherited-conflict') {
            $before = Get-DeploymentKey (Read-TestState $stateDir $scenario.id)
            Write-Utf8 $configPath (New-TestConfig $name $scenario.id $scenario.app $Port $paths $false)
            if ($scenario.effective) { [void](Invoke-Cli 'check' -Reject) } else { [void](Invoke-Cli 'check') }
            Assert-DeploymentUnchanged $before
            Write-Utf8 $configPath $normalConfig
            $manager = New-Object Microsoft.Web.Administration.ServerManager
            try {
                Add-TestHeaderRule ($manager.GetApplicationHostConfiguration()) $name ('rumable-' + $scenario.id + '-inherited-conflict') 'RESPONSE_X_Rumable_Conflict' 'test'
                $manager.CommitChanges()
            } finally { $manager.Dispose() }
            try { [void](Invoke-Cli 'check' -Reject); Assert-DeploymentUnchanged $before }
            finally {
                $manager = New-Object Microsoft.Web.Administration.ServerManager
                try {
                    $rules = $manager.GetApplicationHostConfiguration().GetSection('system.webServer/rewrite/outboundRules', $name).GetCollection()
                    for ($i=$rules.Count-1; $i -ge 0; $i--) { if ($rules[$i].GetAttributeValue('name') -eq ('rumable-' + $scenario.id + '-inherited-conflict')) { $rules.RemoveAt($i) } }
                    $manager.CommitChanges()
                } finally { $manager.Dispose() }
            }
        }
        if ($scenario.id -eq 'inherited') {
            Invoke-Step 'inherited/cache-and-reserved-mapping-conflicts' {
                $before = Get-DeploymentKey (Read-TestState $stateDir $scenario.id)
                $manager = New-Object Microsoft.Web.Administration.ServerManager
                try {
                    $manager.GetApplicationHostConfiguration().GetSection('system.webServer/rewrite/outboundRules', ($name + $scenario.app)).SetAttributeValue('rewriteBeforeCache', $true)
                    $manager.CommitChanges()
                } finally { $manager.Dispose() }
                try { [void](Invoke-Cli 'check' -Reject); Assert-DeploymentUnchanged $before }
                finally {
                    $manager = New-Object Microsoft.Web.Administration.ServerManager
                    try {
                        $manager.GetApplicationHostConfiguration().GetSection('system.webServer/rewrite/outboundRules', ($name + $scenario.app)).GetAttribute('rewriteBeforeCache').Delete()
                        $manager.CommitChanges()
                    } finally { $manager.Dispose() }
                }
                $collision = Join-Path $work 'collision'
                [void][IO.Directory]::CreateDirectory($collision)
                $manager = New-Object Microsoft.Web.Administration.ServerManager
                try {
                    [void]$manager.Sites[$name].Applications[$scenario.app].VirtualDirectories.Add('/__rumable__', $collision)
                    $manager.CommitChanges()
                } finally { $manager.Dispose() }
                try { [void](Invoke-Cli 'check' -Reject); Assert-DeploymentUnchanged $before }
                finally {
                    $manager = New-Object Microsoft.Web.Administration.ServerManager
                    try {
                        $directories = $manager.Sites[$name].Applications[$scenario.app].VirtualDirectories
                        Assert-That ($directories['/__rumable__'].PhysicalPath -eq $collision) 'Reserved mapping was overwritten.'
                        $directories.Remove($directories['/__rumable__'])
                        $manager.CommitChanges()
                    } finally { $manager.Dispose() }
                }
            }
        }
        Invoke-Step ($scenario.id + '/enable-idempotence-html-and-assets') {
            [void](Invoke-Cli 'check')
            $initialBinary = $Binary; if ($PreviousBinary) { $initialBinary = $PreviousBinary }
            [void](Invoke-Cli 'enable' -Exe $initialBinary)
            $enabled = Read-TestState $stateDir $scenario.id
            $key = Get-DeploymentKey $enabled
            [void](Invoke-Cli 'enable' -Exe $initialBinary)
            Assert-DeploymentUnchanged $key
            foreach ($path in $paths) {
                $page = Assert-TestPage $baseUrl $path $scenario.id $enabled $true $parentValue
                if ($path.EndsWith('/chunked.aspx')) { Assert-That ($page.headers['Transfer-Encoding'] -match '(?i)chunked') 'Dynamic fixture did not produce an actual chunked response.' }
            }
            $effective = Get-TestEffective $name $scenario.app
            Assert-That (-not $effective.static -and -not $effective.dynamic -and -not $effective.rewriteBeforeCache) 'Enable changed compression/cache contract.'
            $page = Invoke-TestHttp ($baseUrl + $scenario.app + '/index.html') 'gzip'
            Assert-That (-not $page.headers['Content-Encoding']) 'Static compression was not disabled.'
            Assert-TestAssets $baseUrl $scenario.id $enabled
            foreach ($leaf in @('/excluded/index.html','/nohead.html','/data.json')) {
                $page = Invoke-TestHttp ($baseUrl + $scenario.app + $leaf)
                $original = [IO.File]::ReadAllBytes((Join-Path $web ($scenario.id + $leaf)))
                Assert-That ($page.status -eq 200 -and (Get-SHA256 $page.bytes) -eq (Get-SHA256 $original)) ('Excluded/non-HTML/no-anchor response was modified: ' + $leaf)
            }
            if (-not $StaticOnly) {
                foreach ($leaf in @('/download.aspx','/data.aspx','/nohead.aspx')) {
                    $page = Invoke-TestHttp ($baseUrl + $scenario.app + $leaf)
                    Assert-That ($page.status -eq 200 -and -not $page.text.Contains('data-rumable=')) ('Dynamic bypass failed: ' + $leaf)
                }
            }
            Assert-That ((Get-TestBusinessRules $name) -ceq $business) 'Enable modified business rules.'
        }
        Invoke-Step ($scenario.id + '/upgrade-rollback-and-unowned-parent-edit') {
            $old = Read-TestState $stateDir $scenario.id
            $script:parentValue = 'parent-updated'
            Set-TestRuleValue $name '' 'acceptance-parent' $parentValue
            $script:business = Get-TestBusinessRules $name
            Write-Utf8 $configPath (New-TestConfig $name $scenario.id $scenario.app $Port $paths $true 'upgraded' $true)
            [void](Invoke-Cli 'enable')
            $upgraded = Read-TestState $stateDir $scenario.id
            Assert-That ($upgraded.current.script -ne $old.current.script) 'Upgrade reused the old bootstrap URL.'
            Assert-TestAssets $baseUrl $scenario.id $old
            [void](Invoke-Cli 'rollback')
            $restored = Read-TestState $stateDir $scenario.id
            Assert-That ($restored.current.script -eq $old.current.script) 'Rollback did not restore the earlier bootstrap.'
            [void](Assert-TestPage $baseUrl $paths[0] $scenario.id $restored $true $parentValue)
            Assert-That ((Get-TestBusinessRules $name) -ceq $business) 'Rollback overwrote an unowned parent edit.'
            Assert-TestAssets $baseUrl $scenario.id $upgraded
            Write-Utf8 $configPath $normalConfig
        }
        Invoke-Step ($scenario.id + '/native-and-asset-drift-refusal') {
            $current = Read-TestState $stateDir $scenario.id
            $key = Get-DeploymentKey $current
            $ruleXml = @($current.current.native.rules | Where-Object { ([xml]$_).DocumentElement.GetAttribute('name') -eq ('rumable-' + $scenario.id + '-inject') }) | Select-Object -First 1
            $originalValue = ([xml]$ruleXml).DocumentElement.action.GetAttribute('value')
            Set-TestRuleValue $name $scenario.app ('rumable-' + $scenario.id + '-inject') ($originalValue + '<!-- external acceptance edit -->')
            try { [void](Invoke-Cli 'disable' -Reject); [void](Invoke-Cli 'rollback' -Reject); Assert-DeploymentUnchanged $key }
            finally { Set-TestRuleValue $name $scenario.app ('rumable-' + $scenario.id + '-inject') $originalValue }
            $leaf = [IO.Path]::GetFileName($current.current.script)
            $assetPath = Join-Path $stateDir ('sites/' + $scenario.id + '/public/' + $scenario.id + '/' + $leaf)
            $bytes = [IO.File]::ReadAllBytes($assetPath)
            try {
                [IO.File]::WriteAllBytes($assetPath, [Text.Encoding]::UTF8.GetBytes('/* external acceptance edit */'))
                [void](Invoke-Cli 'disable' -Reject)
                [void](Invoke-Cli 'rollback' -Reject)
                Assert-DeploymentUnchanged $key
                Assert-That ([IO.File]::ReadAllText($assetPath) -eq '/* external acceptance edit */') 'Drift refusal overwrote the external asset.'
            } finally { [IO.File]::WriteAllBytes($assetPath, $bytes) }
        }
        if (-not $StaticOnly) {
            Invoke-Step ($scenario.id + '/compressed-source-rejection-and-post-commit-recovery') {
                $before = Get-DeploymentKey (Read-TestState $stateDir $scenario.id)
                Write-Utf8 $configPath (New-TestConfig $name $scenario.id $scenario.app $Port @($scenario.app + '/gzip.aspx'))
                [void](Invoke-Cli 'enable' -Reject)
                Assert-DeploymentUnchanged $before
                Write-Utf8 (Join-Path $web ($scenario.id + '/data/fail-counter.txt')) '0'
                Write-Utf8 $configPath (New-TestConfig $name $scenario.id $scenario.app $Port @($scenario.app + '/fail.aspx') $true 'must-rollback')
                [void](Invoke-Cli 'enable' -Reject)
                Assert-DeploymentUnchanged $before
                Assert-That ([IO.File]::ReadAllText((Join-Path $web ($scenario.id + '/data/fail-counter.txt'))) -eq '2') 'Failure fixture never reached the post-commit deployment probe.'
                $status = (Invoke-Cli 'status').output | ConvertFrom-Json
                Assert-That (-not $status.pendingTransaction -and -not $status.drift) 'Failure recovery left IIS drift or a pending transaction.'
                [void](Invoke-Cli 'probe')
                [void](Assert-TestPage $baseUrl $paths[0] $scenario.id (Read-TestState $stateDir $scenario.id) $true $parentValue)
                Write-Utf8 $configPath $normalConfig
            }
        }
        Invoke-Step ($scenario.id + '/disable-restore-and-rollback-enable') {
            $old = Read-TestState $stateDir $scenario.id
            [void](Invoke-Cli 'disable')
            [void](Invoke-Cli 'disable')
            $local = Get-TestLocalCompression $name $scenario.app
            Assert-That ($local.present -eq $scenario.present -and $local.value -eq $scenario.value) 'Disable failed to restore exact local compression/inheritance.'
            Assert-That ((Get-TestEffective $name $scenario.app).static -eq $scenario.effective) 'Disable failed to restore effective compression.'
            Assert-TestStaticGzip ($baseUrl + $scenario.app + '/index.html') $scenario.effective
            [void](Assert-TestPage $baseUrl $paths[0] $scenario.id (Read-TestState $stateDir $scenario.id) $false $parentValue)
            Assert-TestAssets $baseUrl $scenario.id $old
            [void](Invoke-Cli 'rollback')
            [void](Invoke-Cli 'probe')
            [void](Assert-TestPage $baseUrl $paths[0] $scenario.id (Read-TestState $stateDir $scenario.id) $true $parentValue)
            [void](Invoke-Cli 'disable')
            $status = (Invoke-Cli 'status').output | ConvertFrom-Json
            Assert-That (-not $status.enabled -and -not $status.pendingTransaction -and -not $status.drift) 'Final status is not disabled and clean.'
            Assert-That ((Get-TestBusinessRules $name) -ceq $business) 'Lifecycle changed unowned rules.'
        }
    }
    Invoke-Step 'multi-application-isolation' {
        foreach ($p in $scenarios) {
            $saved = Read-TestState $stateDir $p.id
            $local = Get-TestLocalCompression $name $p.app
            Assert-That (-not $saved.current.enabled -and $local.present -eq $p.present -and $local.value -eq $p.value) 'One application changed another application state/compression.'
            Assert-TestAssets $baseUrl $p.id $saved
        }
        $root = Get-TestEffective $name ''
        Assert-That ($root.static -and -not $root.dynamic) 'Child deployment changed the parent compression baseline.'
    }
    if ($StaticOnly) {
        $report.steps += @{ name='dynamic-chunked-compressed-source-post-commit'; result='skipped'; reason='StaticOnly requested' }
        $report.result = 'partial'
    } else { $report.result = 'passed' }
} catch {
    $report.result = 'failed'
    $report.error = $_.Exception.Message
    [Console]::Error.WriteLine($_.Exception.Message)
} finally {
    try {
        if (Test-Path -LiteralPath $stateDir) { Copy-Item -LiteralPath $stateDir -Destination (Join-Path $ResultDir 'deployment') -Recurse }
        if ($created) {
            [xml]$xml = [IO.File]::ReadAllText((Join-Path $env:SystemRoot 'System32/inetsrv/config/applicationHost.config'))
            $owned = @($xml.configuration.location | Where-Object { $_.path -ieq $name -or $_.path.StartsWith($name + '/', [StringComparison]::OrdinalIgnoreCase) } | ForEach-Object { $_.OuterXml })
            Write-Utf8 (Join-Path $ResultDir 'test-site-locations.xml') ('<locations>' + ($owned -join '') + '</locations>')
        }
    } catch { $report.artifactError = $_.Exception.Message; $report.result = 'failed' }
    try {
        if ($Keep -and $created) {
            $report.cleanup = @{ result='kept'; site=$name; pool=$name; directory=$work }
        } else {
            if ($created) {
                Remove-TestDeployment $name $work
            }
            elseif (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
            $report.cleanup = @{ result='removed' }
        }
    } catch {
        $report.cleanup = @{ result='failed'; error=$_.Exception.Message; site=$name; directory=$work }
        $report.result = 'failed'
    }
    $report.completedAt = [DateTime]::UtcNow.ToString('o')
    Write-Utf8 (Join-Path $ResultDir 'report.json') ($report | ConvertTo-Json -Depth 60)
    Write-Host ('Result: ' + $report.result + '; report: ' + (Join-Path $ResultDir 'report.json'))
}
if ($report.result -eq 'failed') { exit 1 }
if ($report.result -eq 'partial') { exit 2 }
exit 0
