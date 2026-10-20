#requires -Version 5.1
param(
    [Parameter(Mandatory=$true)][string]$Binary,
    [Parameter(Mandatory=$true)][string]$AgentPath,
    [Parameter(Mandatory=$true)][string]$Endpoint,
    [ValidateSet('datakit','dataway')][string]$Backend='datakit',
    [string]$TokenEnv='RUMABLE_DATAWAY_TOKEN',
    [ValidateRange(1024,65535)][int]$Port=19780,
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
$name='Rumable-OTel-'+[guid]::NewGuid().ToString('N').Substring(0,12)
$work=Join-Path $env:TEMP $name
$web=Join-Path $work 'www';$state=Join-Path $work 'state';$configFile=Join-Path $work 'otel.toml';$log=Join-Path $work 'cli.log'
$created=$false
function Cli([string[]]$Arguments,[switch]$Reject) { return (Invoke-TestCli $Binary ($Arguments+@('--config',$configFile,'--state-dir',$state)) $log -ExpectFailure:$Reject) }
function Assert-Healthy { Assert-That ((Invoke-TestHttp ('http://127.0.0.1:'+$Port+'/index.aspx')).status -eq 200) 'Target application is not healthy.' }
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
    Write-Utf8 (Join-Path $web 'index.aspx') @'
<%@ Page Language="C#" %><script runat="server">
protected void Page_Load(object sender, EventArgs e) {
 if (System.Environment.GetEnvironmentVariable("OTEL_SERVICE_NAME")=="force-probe-fail") Response.StatusCode=503;
}
</script><html><head><title>IIS OTel acceptance</title></head><body>OK</body></html>
'@
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {
        foreach ($existing in $manager.Sites) { foreach ($binding in $existing.Bindings) { Assert-That (($binding.BindingInformation -split ':')[-2] -ne [string]$Port) 'Test port already has an IIS binding.' } }
        $pool=$manager.ApplicationPools.Add($name);$pool.ManagedRuntimeVersion='v4.0';$pool.ManagedPipelineMode='Integrated';$pool.Enable32BitAppOnWin64=[bool]$X86
        $site=$manager.Sites.Add($name,'http',('127.0.0.1:'+$Port+':'),$web);$site.Applications['/'].ApplicationPoolName=$name
        $collection=$pool.GetCollection('environmentVariables');$entry=$collection.CreateElement('add');$entry.SetAttributeValue('name','BUSINESS_KEEP');$entry.SetAttributeValue('value','retained');$collection.Add($entry)
        $manager.GetApplicationHostConfiguration().GetSection('system.webServer/urlCompression',$name).SetAttributeValue('doStaticCompression',$true)
        $manager.CommitChanges();$created=$true
    } finally {$manager.Dispose()}
    $identity='IIS AppPool\'+$name
    & icacls $work /grant ($identity+':(X)') | Out-Null
    & icacls $web /grant ($identity+':(OI)(CI)(RX)') /T | Out-Null
    if ($LASTEXITCODE -ne 0) {throw 'Unable to grant the isolated application pool read access.'}
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
[sites.rum]
enabled = false
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
    Cli @('enable') | Out-Null
    $before=Read-TestState $state 'otel-test';$key=Get-DeploymentKey $before
    Assert-That ($before.current.otelEnabled -and -not $before.current.enabled) 'OTel-only activation changed the RUM switch.'
    Assert-That ($before.current.native.compressionValue -eq 'true') 'OTel-only activation changed static compression.'
    Assert-That ($null -eq $before.current.native.virtualDirectory) 'OTel-only activation created a RUM virtual directory.'
    Assert-Healthy
    Cli @('enable') | Out-Null
    Assert-That ((Get-DeploymentKey (Read-TestState $state 'otel-test')) -ceq $key) 'Repeated enable changed history.'
    Write-Utf8 $configFile ($config.Replace("serviceName = 'iis-otel-acceptance'","serviceName = 'force-probe-fail'"))
    Cli @('enable') -Reject | Out-Null
    Assert-That ((Get-DeploymentKey (Read-TestState $state 'otel-test')) -ceq $key) 'Post-commit HTTP failure did not restore the deployment.'
    Write-Utf8 $configFile $config
    Assert-Healthy
    Edit-Service 'external-change'
    Cli @('disable') -Reject | Out-Null
    Edit-Service 'iis-otel-acceptance'
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$extra=$manager.Sites[$name].Applications.Add('/shared',$web);$extra.ApplicationPoolName=$name;$manager.CommitChanges()} finally {$manager.Dispose()}
    Cli @('enable') -Reject | Out-Null
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$manager.Sites[$name].Applications.Remove($manager.Sites[$name].Applications['/shared']);$manager.CommitChanges()} finally {$manager.Dispose()}
    Cli @('disable','--component','otel') | Out-Null
    $disabled=Read-TestState $state 'otel-test'
    Assert-That (-not $disabled.current.otelEnabled -and @($disabled.current.native.otel.environment.PSObject.Properties).Count -eq 0) 'Disable retained profiler environment.'
    Cli @('rollback') | Out-Null
    Assert-Healthy
    $trace=[guid]::NewGuid().ToString('N')
    $response=Invoke-WebRequest -UseBasicParsing -Uri ('http://127.0.0.1:'+$Port+'/index.aspx') -Headers @{traceparent=('00-'+$trace+'-0123456789abcdef-01')}
    Assert-That ($response.StatusCode -eq 200) 'Instrumented application request failed.'
    $manager=New-Object Microsoft.Web.Administration.ServerManager
    try {$foreign=@($manager.ApplicationPools[$name].GetCollection('environmentVariables') | Where-Object {$_.GetAttributeValue('name') -eq 'BUSINESS_KEEP'});Assert-That ($foreign.Count -eq 1 -and $foreign[0].GetAttributeValue('value') -eq 'retained') 'Business environment changed.'} finally {$manager.Dispose()}
    Cli @('disable') | Out-Null
    Write-Host ('PASS: pool/configuration/HTTP checks. Search trace ID '+$trace+' in Guance to complete ingestion acceptance. Log: '+$log)
} finally {
    if ($created -and -not $Keep) { Remove-TestDeployment $name $work }
    if ($Keep) { Write-Host ('Kept isolated site/pool '+$name+' and '+$work+'; remove after review. State contains server credentials; keep its ACL restricted.') }
}
