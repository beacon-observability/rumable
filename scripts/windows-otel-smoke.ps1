#requires -Version 5.1
param(
    [Parameter(Mandatory=$true)][string]$Binary,
    [Parameter(Mandatory=$true)][string]$AgentPath,
    [Parameter(Mandatory=$true)][string]$Endpoint,
    [ValidateSet('datakit','dataway')][string]$Backend='datakit',
    [string]$TokenEnv='RUMABLE_DATAWAY_TOKEN',
    [ValidateRange(1024,65534)][int]$Port=19780,
    [string]$ResultDir,
    [switch]$WithRUM,
    [switch]$X86,
    [switch]$Keep
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'windows-iis-helpers.ps1')
Assert-That ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT -and [Environment]::Is64BitProcess) 'Run on Windows in elevated 64-bit Windows PowerShell 5.1.'
Assert-That ($PSVersionTable.PSEdition -ne 'Core') 'Use Windows PowerShell 5.1, not PowerShell 7.'
$principal=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
Assert-That ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) 'Administrator required.'
$iis=Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\InetStp'
Assert-That ($iis.MajorVersion -ge 10) 'This OTel suite requires IIS 10; legacy IIS remains RUM-only.'
Add-Type -Path (Join-Path $env:SystemRoot 'System32/inetsrv/Microsoft.Web.Administration.dll')
$Binary=(Resolve-Path -LiteralPath $Binary).Path
$AgentPath=(Resolve-Path -LiteralPath $AgentPath).Path
$name='Rumable-Acceptance-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8)
$peerName='Rumable-Acceptance-'+(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[guid]::NewGuid().ToString('N').Substring(0,8)
$work=Join-Path $env:TEMP $name
$peerWork=Join-Path $env:TEMP $peerName;$peerWeb=Join-Path $peerWork 'www'
foreach ($path in @($work,$peerWork)) {Assert-That (-not (Test-Path -LiteralPath $path)) 'Temporary test path already exists; refusing to reuse it.'}
if ([string]::IsNullOrEmpty($ResultDir)) {$ResultDir=Join-Path $env:TEMP ($name+'-results')}
$ResultDir=[IO.Path]::GetFullPath($ResultDir)
foreach ($tree in @($work,$peerWork)) {Assert-That ($ResultDir -ine $tree -and -not $ResultDir.StartsWith($tree+'\',[StringComparison]::OrdinalIgnoreCase)) 'ResultDir must be outside the temporary sites so cleanup preserves evidence.'}
Assert-That (-not (Test-Path -LiteralPath $ResultDir)) 'Use a new ResultDir to preserve earlier evidence.'
[void][IO.Directory]::CreateDirectory($ResultDir)
$web=Join-Path $work 'www';$state=Join-Path $work 'state';$configFile=Join-Path $work 'otel.toml';$log=Join-Path $ResultDir 'cli.log'
$created=$false
$failure=$null;$steps=New-Object Collections.ArrayList
$report=[ordered]@{result='failed';iis=$iis.VersionString;os=[Environment]::OSVersion.VersionString;ps=[string]$PSVersionTable.PSVersion;binary=$Binary;binarySHA256=(Get-SHA256 ([IO.File]::ReadAllBytes($Binary)));site=$name;peer=$peerName;withRUM=[bool]$WithRUM;x86Pool=[bool]$X86;steps=$steps;traceId=$null;cleanup='pending'}
$savedToken=[Environment]::GetEnvironmentVariable($TokenEnv)
function Cli([string[]]$Arguments,[switch]$Reject) { return (Invoke-TestCli $Binary ($Arguments+@('--config',$configFile,'--state-dir',$state)) $log -ExpectFailure:$Reject) }
function Assert-Healthy { Assert-That ((Invoke-TestHttp ('http://127.0.0.1:'+$Port+'/index.aspx')).status -eq 200) 'Target application is not healthy.' }
function Record-Step([string]$Name) { [void]$steps.Add(@{name=$Name;result='passed';time=[DateTime]::UtcNow.ToString('o')}) }
function Read-App([int]$TargetPort=$Port) {
    $page=Invoke-TestHttp ('http://127.0.0.1:'+$TargetPort+'/index.aspx')
    Assert-That ($page.status -eq 200) 'Application is not healthy.'
    $match=[regex]::Match($page.text,'OTEL:([^;]*);PID:([0-9]+)')
    Assert-That $match.Success 'Application process/environment evidence is missing.'
    return @{service=$match.Groups[1].Value;pid=$match.Groups[2].Value;text=$page.text}
}
function Assert-Service([string]$Expected) { Assert-That ((Read-App).service -ceq $Expected) 'Application still serves another profiler/exporter generation.' }
function Pool-Key([string]$PoolName) {
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {return (@($manager.ApplicationPools[$PoolName].GetCollection('environmentVariables') | ForEach-Object {$_.GetAttributeValue('name')+'='+$_.GetAttributeValue('value')} | Sort-Object) -join "`n")}
    finally {$manager.Dispose()}
}
function Edit-Service([string]$Value) {
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {
        $entries=$manager.ApplicationPools[$name].GetCollection('environmentVariables')
        $entry=@($entries | Where-Object { $_.GetAttributeValue('name') -eq 'OTEL_SERVICE_NAME' })[0]
        $entry.SetAttributeValue('value',$Value);$manager.CommitChanges()
    } finally {$manager.Dispose()}
}
try {
    [void][IO.Directory]::CreateDirectory($web)
    Write-Utf8 $log ''
    $versionResult=Invoke-TestCli $Binary @('version') $log
    $report.binaryVersion=$versionResult.output | ConvertFrom-Json
    $fixture=@'
<%@ Page Language="C#" %><script runat="server">
protected void Page_Load(object sender, EventArgs e) {
 if (System.Environment.GetEnvironmentVariable("OTEL_SERVICE_NAME")=="force-probe-fail") Response.StatusCode=503;
}
</script><html><head><title>IIS OTel acceptance</title></head><body>OTEL:<%= Server.HtmlEncode(System.Environment.GetEnvironmentVariable("OTEL_SERVICE_NAME")) %>;PID:<%= System.Diagnostics.Process.GetCurrentProcess().Id %></body></html>
'@
    Write-Utf8 (Join-Path $web 'index.aspx') $fixture
    Write-Utf8 (Join-Path $peerWeb 'index.aspx') $fixture
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {
        foreach ($existing in $manager.Sites) { foreach ($binding in $existing.Bindings) { Assert-That (@([string]$Port,[string]($Port+1)) -notcontains ($binding.BindingInformation -split ':')[-2]) 'Test port already has an IIS binding.' } }
        $pool=$manager.ApplicationPools.Add($name);$pool.ManagedRuntimeVersion='v4.0';$pool.ManagedPipelineMode='Integrated';$pool.Enable32BitAppOnWin64=[bool]$X86
        $site=$manager.Sites.Add($name,'http',('127.0.0.1:'+$Port+':'),$web);$site.Applications['/'].ApplicationPoolName=$name
        $collection=$pool.GetCollection('environmentVariables');$entry=$collection.CreateElement('add');$entry.SetAttributeValue('name','BUSINESS_KEEP');$entry.SetAttributeValue('value','retained');$collection.Add($entry)
        $manager.GetApplicationHostConfiguration().GetSection('system.webServer/urlCompression',$name).SetAttributeValue('doStaticCompression',$true)
        $peerPool=$manager.ApplicationPools.Add($peerName);$peerPool.ManagedRuntimeVersion='v4.0';$peerPool.ManagedPipelineMode='Integrated'
        $peerSite=$manager.Sites.Add($peerName,'http',('127.0.0.1:'+($Port+1)+':'),$peerWeb);$peerSite.Applications['/'].ApplicationPoolName=$peerName
        foreach ($target in @($name,$peerName)) {$manager.GetApplicationHostConfiguration().GetSection('system.webServer/security/authentication/anonymousAuthentication',$target).SetAttributeValue('userName','')}
        $created=$true;$manager.CommitChanges()
    } finally {$manager.Dispose()}
    $identity='IIS AppPool\'+$name
    & icacls $work /grant ($identity+':(X)') | Out-Null
    & icacls $web /grant ($identity+':(OI)(CI)(RX)') /T | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Unable to grant the isolated application pool read access.'}
    & icacls $peerWork /grant ('IIS AppPool\'+$peerName+':(X)') | Out-Null
    & icacls $peerWeb /grant ('IIS AppPool\'+$peerName+':(OI)(CI)(RX)') /T | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Unable to grant the peer application pool read access.'}
    $peerBaseline=Read-App ($Port+1);$peerKey=Pool-Key $peerName
    Assert-That ($peerBaseline.service -eq '') 'Peer test pool inherited instrumentation; review global service/machine settings first.'
    $token='';if ($Backend -eq 'dataway') {$token="tokenEnv = '$TokenEnv'"}
    $config=@"
schemaVersion = 1
[[sites]]
id = 'otel-test'
publicURL = 'http://127.0.0.1:$Port/'
probePaths = ['/index.aspx']
includePaths = ['/']
[sites.iis]
site = '$name'
application = '/'
allowCompressionChanges = true
[sites.rum]
enabled = false
applicationId = 'rumable-iis-otel-acceptance'
datakitOrigin = 'http://127.0.0.1:1'
sessionSampleRate = 0
telemetrySampleRate = 0
[sites.otel]
enabled = true
backend = '$Backend'
endpoint = '$Endpoint'
serviceName = 'iis-otel-acceptance'
agentPath = '$AgentPath'
runtime = 'netframework'
sampleRate = 100
$token
"@
    Write-Utf8 $configFile $config
    Cli @('render') | Out-Null
    if ($WithRUM) {
        foreach ($path in @($state,(Join-Path $state 'sites'),(Join-Path $state 'sites/otel-test'))) {
            & icacls $path /grant ($identity+':(X)') | Out-Null
            if ($LASTEXITCODE -ne 0) {throw 'Unable to grant public resource parent traversal.'}
        }
        & icacls (Join-Path $state 'sites/otel-test/public') /grant ($identity+':(OI)(CI)(RX)') /T | Out-Null
        if ($LASTEXITCODE -ne 0) {throw 'Unable to grant public resource read access.'}
    }
    Cli @('enable') | Out-Null
    $before=Read-TestState $state 'otel-test';$key=Get-DeploymentKey $before
    Assert-That ($before.current.otelEnabled -and -not $before.current.enabled) 'OTel-only activation changed the RUM switch.'
    Assert-That ($before.current.native.compressionValue -eq 'true') 'OTel-only activation changed static compression.'
    Assert-That ($null -eq $before.current.native.virtualDirectory) 'OTel-only activation created a RUM virtual directory.'
    Assert-Healthy
    Assert-Service 'iis-otel-acceptance'
    Record-Step 'OTel-only activation and live environment'
    $worker=(Read-App).pid
    Cli @('enable') | Out-Null
    Assert-That ((Get-DeploymentKey (Read-TestState $state 'otel-test')) -ceq $key) 'Repeated enable changed history.'
    Assert-That ((Read-App).pid -eq $worker) 'Repeated enable recycled the worker process.'
    Record-Step 'Repeated enable preserves history and worker'
    if ($WithRUM) {
        $otelKey=Pool-Key $name
        try {
            # RUM-only operations must use the deployed credentials, even if
            # the CLI no longer has Dataway credentials in its environment.
            if ($Backend -eq 'dataway') {[Environment]::SetEnvironmentVariable($TokenEnv,$null)}
            Cli @('enable','--component','rum') | Out-Null
            Assert-That ((Pool-Key $name) -ceq $otelKey -and (Read-App).pid -eq $worker) 'Enabling RUM changed OTel environment or recycled its pool.'
            Assert-That ([regex]::Matches((Read-App).text,'data-rumable="otel-test"').Count -eq 1) 'RUM component did not inject exactly once.'
            Assert-TestAssets ('http://127.0.0.1:'+$Port) 'otel-test' (Read-TestState $state 'otel-test')
            Cli @('disable','--component','rum') | Out-Null
            Assert-That ((Pool-Key $name) -ceq $otelKey -and (Read-App).pid -eq $worker) 'Disabling RUM changed OTel environment or recycled its pool.'
        } finally {[Environment]::SetEnvironmentVariable($TokenEnv,$savedToken)}
        Cli @('enable','--component','rum') | Out-Null
        Cli @('disable','--component','otel') | Out-Null
        Assert-Service ''
        Assert-That ([regex]::Matches((Read-App).text,'data-rumable="otel-test"').Count -eq 1) 'Disabling OTel removed RUM injection.'
        Cli @('enable','--component','otel') | Out-Null
        Assert-Service 'iis-otel-acceptance'
        Cli @('disable','--component','rum') | Out-Null
        Assert-That ([regex]::Matches((Read-App).text,'data-rumable="otel-test"').Count -eq 0) 'RUM injection survived component disable.'
        Record-Step 'Independent RUM/OTel switches and deployed credential preservation'
    }
    Write-Utf8 $configFile ($config.Replace('sampleRate = 100','sampleRate = 0'))
    Cli @('enable','--component','otel') | Out-Null
    Assert-That ((Read-TestState $state 'otel-test').current.native.otel.environment.OTEL_TRACES_SAMPLER_ARG -ceq '0') 'Zero sampling did not reach the pool environment.'
    Write-Utf8 $configFile $config
    Cli @('enable','--component','otel') | Out-Null
    Assert-That ((Read-TestState $state 'otel-test').current.native.otel.environment.OTEL_TRACES_SAMPLER_ARG -ceq '1') 'Full sampling was not restored.'
    Record-Step 'Sampling zero/full environment propagation'
    $key=Get-DeploymentKey (Read-TestState $state 'otel-test')
    Write-Utf8 $configFile ($config.Replace("serviceName = 'iis-otel-acceptance'","serviceName = 'force-probe-fail'"))
    Cli @('enable') -Reject | Out-Null
    Assert-That ((Get-DeploymentKey (Read-TestState $state 'otel-test')) -ceq $key) 'Post-commit HTTP failure did not restore the deployment.'
    Write-Utf8 $configFile $config
    Assert-Healthy
    Assert-Service 'iis-otel-acceptance'
    Assert-That (-not (Test-Path -LiteralPath (Join-Path $state 'sites/otel-test/transaction.json'))) 'Recovered deployment retained a transaction.'
    Record-Step 'Post-commit HTTP failure restores process, configuration and history'
    Edit-Service 'external-change'
    Cli @('disable') -Reject | Out-Null
    Edit-Service 'iis-otel-acceptance'
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {
        $entries=$manager.ApplicationPools[$name].GetCollection('environmentVariables');$entry=$entries.CreateElement('add')
        $entry.SetAttributeValue('name','OTEL_EXPORTER_OTLP_TRACES_ENDPOINT');$entry.SetAttributeValue('value','http://127.0.0.1:1');$entries.Add($entry);$manager.CommitChanges()
    } finally {$manager.Dispose()}
    Cli @('disable') -Reject | Out-Null
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {
        $entries=$manager.ApplicationPools[$name].GetCollection('environmentVariables')
        $entry=@($entries | Where-Object {$_.GetAttributeValue('name') -eq 'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT'})[0]
        Assert-That ($entry.GetAttributeValue('value') -eq 'http://127.0.0.1:1') 'External signal override was overwritten.'
        $entries.Remove($entry);$manager.CommitChanges()
    } finally {$manager.Dispose()}
    Record-Step 'External owned environment drift refuses overwrite'
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$extra=$manager.Sites[$name].Applications.Add('/shared',$web);$extra.ApplicationPoolName=$name;$manager.CommitChanges()} finally {$manager.Dispose()}
    Cli @('enable') -Reject | Out-Null
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$manager.Sites[$name].Applications.Remove($manager.Sites[$name].Applications['/shared']);$manager.CommitChanges()} finally {$manager.Dispose()}
    Record-Step 'Shared application pool refuses activation'
    Cli @('disable','--component','otel') | Out-Null
    $disabled=Read-TestState $state 'otel-test'
    Assert-That (-not $disabled.current.otelEnabled -and @($disabled.current.native.otel.environment.PSObject.Properties).Count -eq 0) 'Disable retained profiler environment.'
    Assert-Service ''
    $disabledKey=Get-DeploymentKey $disabled;$disabledWorker=(Read-App).pid
    Cli @('disable','--component','otel') | Out-Null
    Assert-That ((Get-DeploymentKey (Read-TestState $state 'otel-test')) -ceq $disabledKey -and (Read-App).pid -eq $disabledWorker) 'Repeated disable changed history or recycled the worker.'
    try {
        if ($Backend -eq 'dataway') {[Environment]::SetEnvironmentVariable($TokenEnv,$null)}
        Cli @('rollback') | Out-Null
    } finally {[Environment]::SetEnvironmentVariable($TokenEnv,$savedToken)}
    Assert-Healthy
    Assert-Service 'iis-otel-acceptance'
    Record-Step 'Disable, repeated disable and saved-credential rollback'
    $trace=[guid]::NewGuid().ToString('N')
    $response=Invoke-WebRequest -UseBasicParsing -Uri ('http://127.0.0.1:'+$Port+'/index.aspx') -Headers @{traceparent=('00-'+$trace+'-0123456789abcdef-01')}
    Assert-That ($response.StatusCode -eq 200) 'Instrumented application request failed.'
    $report.traceId=$trace
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$foreign=@($manager.ApplicationPools[$name].GetCollection('environmentVariables') | Where-Object {$_.GetAttributeValue('name') -eq 'BUSINESS_KEEP'});Assert-That ($foreign.Count -eq 1 -and $foreign[0].GetAttributeValue('value') -eq 'retained') 'Business environment changed.'} finally {$manager.Dispose()}
    Cli @('disable') | Out-Null
    Assert-Service ''
    $peerAfter=Read-App ($Port+1)
    Assert-That ((Pool-Key $peerName) -ceq $peerKey -and $peerAfter.pid -eq $peerBaseline.pid -and $peerAfter.service -eq '') 'An unrelated application pool was changed, recycled or instrumented.'
    Record-Step 'Unmanaged peer pool environment and worker preserved'
    $report.result='passed'
} catch {
    $failure=$_;$report.error=$_.Exception.Message
} finally {
    [Environment]::SetEnvironmentVariable($TokenEnv,$savedToken)
    if ($created -and -not $Keep) {
        $report.cleanup='passed'
        foreach ($deployment in @(@{name=$name;work=$work},@{name=$peerName;work=$peerWork})) {
            try {Remove-TestDeployment $deployment.name $deployment.work}
            catch {$report.cleanup='failed';$report.result='failed';$report.cleanupError=$_.Exception.Message;if ($null -eq $failure) {$failure=$_}}
        }
    } elseif ($Keep) {
        $report.cleanup='kept'
        Write-Host ('Kept isolated site/pool '+$name+' in '+$work+' and peer '+$peerName+' in '+$peerWork+'. State contains server credentials; keep its ACL restricted.')
    } else {$report.cleanup='not-created'}
    Write-Utf8 (Join-Path $ResultDir 'report.json') ($report | ConvertTo-Json -Depth 20)
}
if ($null -ne $failure) {throw $failure}
Write-Host ('PASS: pool/configuration/process/HTTP checks. Search trace ID '+$trace+' in Guance to complete ingestion acceptance. Report: '+(Join-Path $ResultDir 'report.json'))
