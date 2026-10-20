# Loaded by the IIS bridge. Only the selected dedicated pool is modified.
$otelKeys = @('OTEL_DOTNET_AUTO_HOME','OTEL_SERVICE_NAME','OTEL_TRACES_EXPORTER','OTEL_METRICS_EXPORTER','OTEL_LOGS_EXPORTER','OTEL_EXPORTER_OTLP_PROTOCOL','OTEL_EXPORTER_OTLP_ENDPOINT','OTEL_EXPORTER_OTLP_HEADERS','OTEL_TRACES_SAMPLER','OTEL_TRACES_SAMPLER_ARG','OTEL_PROPAGATORS','COR_ENABLE_PROFILING','COR_PROFILER','COR_PROFILER_PATH_32','COR_PROFILER_PATH_64','CORECLR_ENABLE_PROFILING','CORECLR_PROFILER','CORECLR_PROFILER_PATH_32','CORECLR_PROFILER_PATH_64','DOTNET_STARTUP_HOOKS','DOTNET_ADDITIONAL_DEPS','DOTNET_SHARED_STORE')
$profilerId = '{918728DD-259F-4A6A-AC2B-B85E1B658318}'

function Get-OTelPool {
    if ($iisVersion.MajorVersion -lt 10 -or $PSVersionTable.PSVersion -lt [version]'5.1' -or [Environment]::OSVersion.Version.Major -lt 10) { throw 'IIS OTel requires IIS 10, Windows 10/Server 2016+ and Windows PowerShell 5.1; legacy IIS remains RUM-only.' }
    $pool = $manager.ApplicationPools[$application.ApplicationPoolName]
    if ($null -eq $pool) { throw 'IIS application pool not found.' }
    return $pool
}

function Read-OTelSnapshot {
    if ($null -eq $site.otel) { return $null }
    $pool = Get-OTelPool
    $values = @{}
    foreach ($entry in $pool.GetCollection('environmentVariables')) {
        $name = [string]$entry.GetAttributeValue('name')
        if ($otelKeys -contains $name) { $values[$name] = [string]$entry.GetAttributeValue('value') }
    }
    $users = @($manager.Sites | ForEach-Object { $owner=$_.Name; $_.Applications | Where-Object { $_.ApplicationPoolName -ieq $pool.Name } | ForEach-Object { $owner + $_.Path } } | Sort-Object)
    return @{ pool=$pool.Name; environment=$values; runtime=$pool.ManagedRuntimeVersion; pipeline=[string]$pool.ManagedPipelineMode; x86=[bool]$pool.Enable32BitAppOnWin64; identity=[string]$pool.ProcessModel.IdentityType; user=[string]$pool.ProcessModel.UserName; applications=$users }
}

function Assert-OTelPool {
    $pool = Get-OTelPool
    $users = @($manager.Sites | ForEach-Object { $_.Applications } | Where-Object { $_.ApplicationPoolName -ieq $pool.Name })
    if ($users.Count -ne 1) { throw 'IIS OTel requires a dedicated application pool; shared pools could instrument other sites/applications.' }
    if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { throw 'IIS OTel ARM64 is not accepted in this release; select Windows x64 with an x64/x86 application pool.' }
    $agent = [string]$site.otel.agentPath
    if (-not [IO.Path]::IsPathRooted($agent)) { throw 'IIS OTel agentPath must be absolute.' }
    $arch = 'win-x64'; if ($pool.Enable32BitAppOnWin64) { $arch = 'win-x86' }
    if (-not (Test-Path -LiteralPath (Join-Path $agent ($arch + '/OpenTelemetry.AutoInstrumentation.Native.dll')) -PathType Leaf)) { throw 'Install the official .NET automatic instrumentation matching the application pool architecture in agentPath.' }
    if ($site.otel.runtime -eq 'netframework') {
        $release = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full').Release
        if ($release -lt 394802 -or $pool.ManagedRuntimeVersion -ne 'v4.0' -or [string]$pool.ManagedPipelineMode -ne 'Integrated') { throw 'IIS OTel netframework requires .NET Framework 4.6.2+ and a v4 Integrated pool.' }
    } elseif (-not (Test-Path -LiteralPath (Join-Path $agent 'net/OpenTelemetry.AutoInstrumentation.StartupHook.dll') -PathType Leaf)) { throw 'The .NET startup hook is missing from agentPath.' }
    foreach ($key in [Environment]::GetEnvironmentVariables('Machine').Keys) {
        if ($key -like 'OTEL_*' -or $otelKeys -contains $key) { throw 'Machine-wide OTel/profiler/startup settings already exist; review their conflict manually.' }
    }
    foreach ($entry in $pool.GetCollection('environmentVariables')) {
        $key = [string]$entry.GetAttributeValue('name')
        if ($key -like 'OTEL_*' -and $otelKeys -notcontains $key) { throw 'Unmanaged pool OTel overrides could bypass the configured exporter/sampler; review them manually.' }
    }
    $live = Read-OTelSnapshot
    $owned = $null
    $state = Join-Path $request.root 'state.json'
    if (Test-Path -LiteralPath $state) { $owned = ([IO.File]::ReadAllText($state) | ConvertFrom-Json).current.native.otel }
    if (Test-Path -LiteralPath $state) {
        $original = ([IO.File]::ReadAllText($state) | ConvertFrom-Json).originalIIS.otel
        if ($null -ne $original -and @($original.environment.PSObject.Properties).Count -gt 0) { throw 'Pre-existing application pool instrumentation settings are not owned by Rumable; refusing to replace them.' }
    }
    if ($live.environment.Count -gt 0 -and $null -eq $owned) { throw 'The application pool already has OTel/profiler environment settings without Rumable ownership.' }
    if ($site.otel.backend -eq 'dataway' -and [string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($site.otel.tokenEnv))) { throw 'The environment variable named by otel.tokenEnv is missing.' }
}

function Same-OTelEnvironment($expected, $actual) {
    if ($expected -is [Collections.IDictionary]) { $names = @($expected.Keys) } else { $names = @($expected.PSObject.Properties.Name) }
    if ($names.Count -ne $actual.Count) { return $false }
    foreach ($key in $names) {
        if (-not $actual.ContainsKey($key) -or $expected.$key -cne $actual.$key) { return $false }
    }
    return $true
}

function Set-OTelEnvironment($values) {
    if (Same-OTelEnvironment $values (Read-OTelSnapshot).environment) { return }
    $pool = Get-OTelPool
    $collection = $pool.GetCollection('environmentVariables')
    for ($i=$collection.Count-1; $i -ge 0; $i--) {
        if ($otelKeys -contains [string]$collection[$i].GetAttributeValue('name')) { $collection.RemoveAt($i) }
    }
    if ($values -is [Collections.IDictionary]) { $names = @($values.Keys) } else { $names = @($values.PSObject.Properties.Name) }
    foreach ($key in $names) {
        if ($otelKeys -notcontains $key) { throw 'Invalid OTel environment key in snapshot.' }
        $entry = $collection.CreateElement('add')
        $entry.SetAttributeValue('name',[string]$key)
        $entry.SetAttributeValue('value',[string]$values.$key)
        [void]$collection.Add($entry)
    }
}

function Restore-OTel($snapshot) {
    if ($null -eq $site.otel) { return }
    $pool = Get-OTelPool
    if ($null -eq $snapshot -or $snapshot.pool -cne $pool.Name) { throw 'OTel snapshot application pool mismatch; refusing to change another pool.' }
    Set-OTelEnvironment $snapshot.environment
}

function Apply-OTel {
    if ($null -eq $site.otel) { return }
    if (-not $request.otelEnabled) { Restore-OTel $request.original.otel; return }
    Assert-OTelPool
    $pool = Get-OTelPool
    $o = $site.otel
    $agent = [string]$o.agentPath
    $protocol = 'grpc'; if ($o.backend -eq 'dataway') { $protocol = 'http/protobuf' }
    $rate = 1.0; if ($null -ne $o.sampleRate) { $rate = [double]$o.sampleRate / 100 }
    $values = @{
        OTEL_DOTNET_AUTO_HOME=$agent; OTEL_SERVICE_NAME=[string]$o.serviceName
        OTEL_TRACES_EXPORTER='otlp'; OTEL_METRICS_EXPORTER='none'; OTEL_LOGS_EXPORTER='none'
        OTEL_EXPORTER_OTLP_PROTOCOL=$protocol; OTEL_EXPORTER_OTLP_ENDPOINT=[string]$o.endpoint
        OTEL_TRACES_SAMPLER='parentbased_traceidratio'; OTEL_TRACES_SAMPLER_ARG=$rate.ToString([Globalization.CultureInfo]::InvariantCulture)
        OTEL_PROPAGATORS='tracecontext,baggage'
    }
    if ($o.backend -eq 'dataway') { $values.OTEL_EXPORTER_OTLP_HEADERS = 'X-Token=' + [uri]::EscapeDataString([Environment]::GetEnvironmentVariable($o.tokenEnv)) }
    $arch = '64'; $dir = 'win-x64'; if ($pool.Enable32BitAppOnWin64) { $arch='32'; $dir='win-x86' }
    $native = Join-Path $agent ($dir + '/OpenTelemetry.AutoInstrumentation.Native.dll')
    if ($o.runtime -eq 'netframework') {
        $values.COR_ENABLE_PROFILING='1'; $values.COR_PROFILER=$profilerId; $values['COR_PROFILER_PATH_' + $arch]=$native
    } else {
        $values.CORECLR_ENABLE_PROFILING='1'; $values.CORECLR_PROFILER=$profilerId; $values['CORECLR_PROFILER_PATH_' + $arch]=$native
        $values.DOTNET_STARTUP_HOOKS=Join-Path $agent 'net/OpenTelemetry.AutoInstrumentation.StartupHook.dll'
        $values.DOTNET_ADDITIONAL_DEPS=Join-Path $agent 'AdditionalDeps'
        $values.DOTNET_SHARED_STORE=Join-Path $agent 'store'
    }
    Set-OTelEnvironment $values
}

function Recycle-OTelPool($before) {
    if ($null -eq $site.otel) { return }
    $after = Read-OTelSnapshot
    if (-not (Same-OTelEnvironment $before.environment $after.environment)) {
        $pool = Get-OTelPool
        if (@($after.applications).Count -ne 1) { throw 'Application pool became shared; refusing to recycle other applications.' }
        if ([string]$pool.State -eq 'Started') { $pool.Recycle() }
    }
}
