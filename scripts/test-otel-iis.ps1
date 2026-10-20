$ErrorActionPreference = 'Stop'
# Portable bridge contract checks. These mocks do not execute IIS or the agent.
. (Join-Path $PSScriptRoot '../internal/rumable/assets/otel-iis.ps1')
$iisVersion = @{ MajorVersion=7; MinorVersion=5 }
try { Get-OTelPool; throw 'Legacy IIS was accepted for OTel.' } catch { if ($_.Exception.Message -notlike '*IIS OTel requires*') { throw } }

function New-Entry {
    $entry = [pscustomobject]@{ Values=@{} }
    $entry | Add-Member ScriptMethod GetAttributeValue { param($name) return $this.Values[$name] }
    $entry | Add-Member ScriptMethod SetAttributeValue { param($name,$value) $this.Values[$name]=$value }
    return $entry
}
class RumableMockCollection : Collections.ArrayList {
    [object] CreateElement([string]$tag) { return (New-Entry) }
}
class RumableMockPool {
    [string]$Name='Dedicated'
    [string]$ManagedRuntimeVersion='v4.0'
    [string]$ManagedPipelineMode='Integrated'
    [bool]$Enable32BitAppOnWin64=$false
    [string]$State='Started'
    [int]$Recycles=0
    [object]$ProcessModel=@{IdentityType='ApplicationPoolIdentity';UserName=''}
    [object[]]$WorkerProcesses=@()
    [RumableMockCollection]$Environment
    [RumableMockCollection] GetCollection([string]$name) { return $this.Environment }
    [string] Recycle() { $this.Recycles++; return 'Started' }
}
$environment = New-Object RumableMockCollection
$pool=New-Object RumableMockPool
$pool.Environment=$environment
function Get-OTelPool { return $pool }
$manager = @{ Sites=@(@{Name='OnlySite';Applications=@(@{Path='/';ApplicationPoolName='Dedicated'})}) }
$appEnvironment=New-Object RumableMockCollection
$section=[pscustomobject]@{}
$section | Add-Member ScriptMethod GetCollection { param($name) return ,$appEnvironment }
$webConfig=[pscustomobject]@{}
$webConfig | Add-Member ScriptMethod GetSection { param($name) return $section }
$manager | Add-Member ScriptMethod GetWebConfiguration { param($name,$path) return $webConfig }
$site = @{ otel=@{agentPath='C:\otel';backend='dataway';endpoint='https://dataway.example.com/otel';serviceName='iis-orders';runtime='netframework';sampleRate=12.5;tokenEnv='RUMABLE_MOCK_TOKEN'} }
$original = Read-OTelSnapshot
$entry=New-Entry;$entry.SetAttributeValue('name','BUSINESS_KEEP');$entry.SetAttributeValue('value','unchanged');[void]$environment.Add($entry)
$work=Join-Path ([IO.Path]::GetTempPath()) ('rumable-otel-contract-'+[guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($work)
$site.otel.agentPath=Join-Path $work 'agent'
$request = @{ root=$work; otelEnabled=$true; original=@{otel=$original} }
foreach ($arch in @('win-x64','win-x86','net')) {[void][IO.Directory]::CreateDirectory((Join-Path $site.otel.agentPath $arch))}
foreach ($arch in @('win-x64','win-x86')) {[IO.File]::WriteAllText((Join-Path $site.otel.agentPath ($arch+'/OpenTelemetry.AutoInstrumentation.Native.dll')),'mock')}
[IO.File]::WriteAllText((Join-Path $site.otel.agentPath 'net/OpenTelemetry.AutoInstrumentation.StartupHook.dll'),'mock')
$services=@{WAS=@();W3SVC=@()}
function Get-ItemProperty { param($Path,$LiteralPath)
    if ($Path -like '*NDP*') { return @{Release=394802} }
    return @{Environment=$services[($LiteralPath -split '\\')[-1]]}
}
function Test-Path { param($LiteralPath,$PathType)
    if ($LiteralPath -like 'HKLM:*') { return $true }
    return (Microsoft.PowerShell.Management\Test-Path @PSBoundParameters)
}
# The rest of the production pool checks run unchanged; only OS APIs are mocked.
function Save-Owned { @{current=@{native=@{otel=(Read-OTelSnapshot)}};originalIIS=@{otel=$original}} | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $work 'state.json') }
function Reject([scriptblock]$Action) { $rejected=$false;try {& $Action}catch {$rejected=$true};if (-not $rejected) {throw 'Unsafe pool configuration was accepted.'} }
[Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN','test token&value')
try {
    $savedArchitecture=$env:PROCESSOR_ARCHITEW6432
    try {$env:PROCESSOR_ARCHITEW6432='ARM64';Reject {Assert-OTelPool}}
    finally {$env:PROCESSOR_ARCHITEW6432=$savedArchitecture}
    $manager.Sites[0].Applications+=@{Path='/other';ApplicationPoolName='Dedicated'}
    Reject {Assert-OTelPool}
    $manager.Sites[0].Applications=@($manager.Sites[0].Applications[0])
    foreach ($service in @('WAS','W3SVC')) {
        $services[$service]=@('BUSINESS_KEEP=yes','COR_PROFILER_PATH=C:\foreign.dll')
        Reject {Assert-OTelPool}
        $services[$service]=@()
    }
    $site.otel.runtime='dotnet'
    Reject {Assert-OTelPool}
    $pool.ManagedRuntimeVersion='';Assert-OTelPool
    $entry=New-Entry;$entry.SetAttributeValue('name','OTEL_SERVICE_NAME');$entry.SetAttributeValue('value','foreign-application');[void]$appEnvironment.Add($entry)
    Reject {Assert-OTelPool}
    $appEnvironment.Clear()
    $pool.ManagedRuntimeVersion='v4.0';$site.otel.runtime='netframework'
    Apply-OTel
    $live=Read-OTelSnapshot
    if ($live.environment.OTEL_EXPORTER_OTLP_PROTOCOL -ne 'http/protobuf' -or $live.environment.OTEL_EXPORTER_OTLP_HEADERS -cne 'X-Token=test%20token%26value') {throw 'Dataway protocol/authentication did not reach the application pool.'}
    if ($live.environment.OTEL_TRACES_SAMPLER_ARG -cne '0.125' -or $live.environment.COR_PROFILER_PATH_64 -notlike '*win-x64*') {throw 'Sampling or target profiler architecture was lost.'}
    Save-Owned
    $keys=@($environment.ToArray())
    Apply-OTel
    if ($environment.Count -ne $keys.Count -or -not [object]::ReferenceEquals($keys[0],$environment[0])) {throw 'Repeated apply rewrote or duplicated pool environment entries.'}
    $output=@(Recycle-OTelPool $live)
    if ($output.Count -ne 0) {throw 'IIS recycle status leaked into the bridge JSON response.'}
    if ($pool.Recycles -ne 0) {throw 'Unchanged settings recycled the application pool.'}
    $reordered=$live.Clone();$reordered.environment=[ordered]@{}
    foreach ($key in ($live.environment.Keys | Sort-Object -Descending)) {$reordered.environment[$key]=$live.environment[$key]}
    Recycle-OTelPool $reordered
    if ($pool.Recycles -ne 0) {throw 'Environment serialization order recycled an unchanged pool.'}
    $request.preserveOTel=$true;$request.expected=@{otel=$live}
    [Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN',$null)
    Apply-OTel
    if (-not (Same-OTelEnvironment $live.environment (Read-OTelSnapshot).environment)) {throw 'RUM-only operation changed OTel credentials.'}
    $request.preserveOTel=$false
    [Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN','rotated token')
    $site.otel.serviceName='changed-service';Apply-OTel
    $output=@(Recycle-OTelPool $live)
    if ($output.Count -ne 0) {throw 'IIS recycle status leaked into the bridge JSON response.'}
    if ($pool.Recycles -ne 1) {throw 'A real exporter environment change did not recycle the pool.'}
    $site.otel.serviceName='iis-orders'
    $site.otel.runtime='dotnet';$pool.Enable32BitAppOnWin64=$true
    $pool.ManagedRuntimeVersion=''
    Apply-OTel
    $core=Read-OTelSnapshot
    if ($core.environment.ContainsKey('COR_PROFILER') -or $core.environment.CORECLR_PROFILER_PATH_32 -notlike '*win-x86*' -or $core.environment.DOTNET_STARTUP_HOOKS -notlike '*StartupHook.dll') {throw 'Runtime/architecture upgrade retained conflicting profiler settings.'}
    $request.otelEnabled=$false
    Apply-OTel
    if ((Read-OTelSnapshot).environment.Count -ne 0 -or $environment.Count -ne 1 -or $environment[0].GetAttributeValue('value') -cne 'unchanged') {throw 'Disable failed to restore the original environment or changed a business key.'}
    $roundtrip=$core | ConvertTo-Json -Depth 20 | ConvertFrom-Json
    [Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN',$null)
    Restore-OTel $roundtrip
    if ((Read-OTelSnapshot).environment.OTEL_SERVICE_NAME -cne 'iis-orders') {throw 'Rollback lost the saved application pool environment.'}
    Assert-OTelPool -SavedCredentials
    Reject {Assert-OTelPool}
    $pool.State='Stopped';Reject {Recycle-OTelPool $original};$pool.State='Started'
    $manager.Sites[0].Applications+=@{Path='/other';ApplicationPoolName='Dedicated'}
    Reject {Recycle-OTelPool $original}
    $manager.Sites[0].Applications=@($manager.Sites[0].Applications[0])
    $process=Start-Process (Get-Process -Id $PID).Path -ArgumentList @('-NoProfile','-Command','Start-Sleep -Seconds 1') -PassThru
    try {
        $pool.WorkerProcesses=@(@{ProcessId=$process.Id})
        Recycle-OTelPool $original
        if (-not $process.HasExited) {throw 'Activation returned before its old worker exited.'}
    } finally { $pool.WorkerProcesses=@();$process.Dispose() }
    $roundtrip.pool='AnotherPool'
    try {Restore-OTel $roundtrip;throw 'Wrong pool was restored.'} catch {if ($_.Exception.Message -notlike '*pool mismatch*') {throw}}
    $services.WAS=@('CORECLR_ENABLE_PROFILING=1')
    $unchanged=Read-OTelSnapshot
    Reject {Restore-OTel $original}
    if (-not (Same-OTelEnvironment $unchanged.environment (Read-OTelSnapshot).environment)) {throw 'Global conflict rejection partially changed pool configuration.'}
    $services.WAS=@()
    $entry=New-Entry;$entry.SetAttributeValue('name','OTEL_EXPORTER_OTLP_TRACES_ENDPOINT');$entry.SetAttributeValue('value','http://foreign');[void]$environment.Add($entry)
    if (-not (Read-OTelSnapshot).environment.ContainsKey('OTEL_EXPORTER_OTLP_TRACES_ENDPOINT')) {throw 'Unmanaged signal override was invisible to drift protection.'}
    Reject {Assert-OTelPool}
    Write-Host 'PASS: IIS OTel contract (mock): dedicated pool, ARM64 emulation rejection, WAS/W3SVC/app overrides, Core CLR mode, semantic idempotency, independent RUM credentials, profiler/runtime switch, worker exit wait and saved-credential rollback.'
} finally {
    [Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN',$null)
    Remove-Item -LiteralPath $work -Recurse -Force
}
