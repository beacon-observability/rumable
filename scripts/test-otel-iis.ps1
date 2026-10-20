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
    [RumableMockCollection]$Environment
    [RumableMockCollection] GetCollection([string]$name) { return $this.Environment }
    [void] Recycle() { $this.Recycles++ }
}
$environment = New-Object RumableMockCollection
$pool=New-Object RumableMockPool
$pool.Environment=$environment
function Get-OTelPool { return $pool }
function Assert-OTelPool { }
$manager = @{ Sites=@(@{Name='OnlySite';Applications=@(@{Path='/';ApplicationPoolName='Dedicated'})}) }
$site = @{ otel=@{agentPath='C:\otel';backend='dataway';endpoint='https://dataway.example.com/otel';serviceName='iis-orders';runtime='netframework';sampleRate=12.5;tokenEnv='RUMABLE_MOCK_TOKEN'} }
$site.otel.agentPath=Join-Path ([IO.Path]::GetTempPath()) 'otel-mock'
$original = Read-OTelSnapshot
$entry=New-Entry;$entry.SetAttributeValue('name','BUSINESS_KEEP');$entry.SetAttributeValue('value','unchanged');[void]$environment.Add($entry)
$request = @{ otelEnabled=$true; original=@{otel=$original} }
[Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN','test token&value')
try {
    Apply-OTel
    $live=Read-OTelSnapshot
    if ($live.environment.OTEL_EXPORTER_OTLP_PROTOCOL -ne 'http/protobuf' -or $live.environment.OTEL_EXPORTER_OTLP_HEADERS -cne 'X-Token=test%20token%26value') {throw 'Dataway protocol/authentication did not reach the application pool.'}
    if ($live.environment.OTEL_TRACES_SAMPLER_ARG -cne '0.125' -or $live.environment.COR_PROFILER_PATH_64 -notlike '*win-x64*') {throw 'Sampling or target profiler architecture was lost.'}
    $keys=@($live.environment.Keys)
    Apply-OTel
    if ((Read-OTelSnapshot).environment.Count -ne $keys.Count) {throw 'Repeated apply duplicated pool environment entries.'}
    Recycle-OTelPool $live
    if ($pool.Recycles -ne 0) {throw 'Unchanged settings recycled the application pool.'}
    $reordered=$live.Clone();$reordered.environment=[ordered]@{}
    foreach ($key in ($live.environment.Keys | Sort-Object -Descending)) {$reordered.environment[$key]=$live.environment[$key]}
    Recycle-OTelPool $reordered
    if ($pool.Recycles -ne 0) {throw 'Environment serialization order recycled an unchanged pool.'}
    $site.otel.serviceName='changed-service';Apply-OTel
    Recycle-OTelPool $live
    if ($pool.Recycles -ne 1) {throw 'A real exporter environment change did not recycle the pool.'}
    $site.otel.serviceName='iis-orders'
    $site.otel.runtime='dotnet';$pool.Enable32BitAppOnWin64=$true
    Apply-OTel
    $core=Read-OTelSnapshot
    if ($core.environment.ContainsKey('COR_PROFILER') -or $core.environment.CORECLR_PROFILER_PATH_32 -notlike '*win-x86*' -or $core.environment.DOTNET_STARTUP_HOOKS -notlike '*StartupHook.dll') {throw 'Runtime/architecture upgrade retained conflicting profiler settings.'}
    $request.otelEnabled=$false
    Apply-OTel
    if ((Read-OTelSnapshot).environment.Count -ne 0 -or $environment.Count -ne 1 -or $environment[0].GetAttributeValue('value') -cne 'unchanged') {throw 'Disable failed to restore the original environment or changed a business key.'}
    $roundtrip=$live | ConvertTo-Json -Depth 20 | ConvertFrom-Json
    Restore-OTel $roundtrip
    if ((Read-OTelSnapshot).environment.OTEL_SERVICE_NAME -cne 'iis-orders') {throw 'Rollback lost the saved application pool environment.'}
    $roundtrip.pool='AnotherPool'
    try {Restore-OTel $roundtrip;throw 'Wrong pool was restored.'} catch {if ($_.Exception.Message -notlike '*pool mismatch*') {throw}}
    Write-Host 'PASS: IIS OTel bridge contract (mock): legacy rejection, protocol/auth, sampling, x86/x64/runtime switch, idempotency, foreign environment preservation and rollback ownership.'
} finally { [Environment]::SetEnvironmentVariable('RUMABLE_MOCK_TOKEN',$null) }
