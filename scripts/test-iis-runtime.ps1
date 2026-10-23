$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
. (Join-Path $PSScriptRoot '../internal/rumable/assets/otel-iis.ps1')
function Assert-That($v,$message) { if (-not $v) { throw $message } }
function Reject([scriptblock]$action,[string]$reason) {
    $rejected=$false
    try { & $action } catch { if ($_.Exception.Message -notlike ('*'+$reason+'*')) { throw };$rejected=$true }
    Assert-That $rejected ('Expected rejection: '+$reason)
}
$pool=[pscustomobject]@{Name='Dedicated';State='Started';WorkerProcesses=@();Recycles=0}
$pool | Add-Member ScriptMethod Recycle { $this.Recycles++;return 'Started' }
$application=@{Path='/app';ApplicationPoolName='Dedicated'}
$site=@{iis=@{site='Website';application='/app'}}
$manager=@{ApplicationPools=@{Dedicated=$pool};Sites=@(@{Name='Website';Applications=@($application)})}
$baseline=Read-RumRuntime
Assert-RumPool
Assert-That (Test-RumRuntime ($baseline | ConvertTo-Json | ConvertFrom-Json) (Read-RumRuntime)) 'Runtime JSON round trip failed'
$result=@(Recycle-OTelPool $null -RumChanged)
Assert-That ($pool.Recycles -eq 1 -and $result.Count -eq 0) 'RUM-only activation was skipped or polluted JSON'
Recycle-OTelPool $null
Assert-That ($pool.Recycles -eq 1) 'Unchanged RUM configuration recycled the pool'

$manager.Sites+=@{Name='Unselected';Applications=@(@{Path='/';ApplicationPoolName='Dedicated'})}
Assert-That (-not (Test-RumRuntime $baseline (Read-RumRuntime))) 'Pool membership drift was ignored'
Reject {Recycle-OTelPool $null -RumChanged} 'dedicated application pool'
Assert-That ($pool.Recycles -eq 1) 'Shared pool was recycled'
$manager.Sites=@($manager.Sites[0])
$pool.State='Stopped'
Reject {Assert-RumPool -RequireStarted} 'not Started'
Reject {Recycle-OTelPool $null -RumChanged} 'not Started'
Assert-That ($pool.Recycles -eq 1) 'Stopped pool was recycled or started'
$pool.State='Started'
$foreign=$baseline.Clone();$foreign.pool='Other'
Assert-That (-not (Test-RumRuntime $foreign $baseline)) 'Pool reassignment was ignored'
Assert-That (Test-RumRuntime $null $baseline) 'Legacy recovery was rejected before checking the live dedicated pool'

# Test the real .NET process wait with short-lived child processes. A timeout
# does not terminate the child; production never kills application workers.
$process=Start-Process (Join-Path $PSHOME 'pwsh') -ArgumentList '-NoProfile','-Command','Start-Sleep -Milliseconds 700' -PassThru
try {
    $pool.WorkerProcesses=@(@{ProcessId=$process.Id})
    $timer=[Diagnostics.Stopwatch]::StartNew()
    $result=@(Recycle-OTelPool $null -RumChanged)
    Assert-That ($process.HasExited -and $timer.ElapsedMilliseconds -ge 300 -and $result.Count -eq 0) 'Activation returned before old worker exit'
} finally { $pool.WorkerProcesses=@();$process.Dispose() }
$process=Start-Process (Join-Path $PSHOME 'pwsh') -ArgumentList '-NoProfile','-Command','Start-Sleep -Milliseconds 700' -PassThru
try {
    Reject {Wait-IISWorkers @($process) 20} 'did not exit'
    Assert-That (-not $process.HasExited) 'Activation timeout killed the worker'
    $process.WaitForExit()
} finally { $process.Dispose() }

# A combined RUM/OTel change must recycle once and still validate profiler
# prerequisites; an unchanged OTel config must not demand its token again.
function Read-OTelSnapshot { return @{environment=@{OTEL_SERVICE_NAME='new'};applications=@('Website/app')} }
function Assert-OTelExternalEnvironment { $script:externalChecks++ }
$script:externalChecks=0;$site.otel=@{};$count=$pool.Recycles
Recycle-OTelPool @{environment=@{OTEL_SERVICE_NAME='old'}} -RumChanged
Assert-That ($pool.Recycles -eq $count+1 -and $script:externalChecks -eq 1) 'Combined activation duplicated recycle or skipped OTel validation'
Recycle-OTelPool @{environment=@{OTEL_SERVICE_NAME='new'}} -RumChanged
Assert-That ($pool.Recycles -eq $count+2 -and $script:externalChecks -eq 1) 'RUM-only update revalidated unchanged OTel credentials'
Write-Host 'PASS: RUM-only/combined activation, shared/stopped pool refusal, membership drift, JSON isolation, old-worker drain and bounded timeout (mock IIS; real process wait).'
