param([Parameter(Mandatory=$true)][string]$RequestPath)
$ErrorActionPreference = 'Stop'
try {
    if ($PSVersionTable.PSVersion.Major -lt 3) { throw 'PowerShell 3.0+ is required (5.1 recommended on legacy Windows).' }
    if (-not [Environment]::Is64BitProcess) { throw 'IIS management requires a 64-bit PowerShell host.' }
    $iisVersion = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\InetStp'
    if ($iisVersion.MajorVersion -lt 7 -or ($iisVersion.MajorVersion -eq 7 -and $iisVersion.MinorVersion -lt 5)) { throw 'IIS 7.5+ is required.' }
    [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
    $request = [IO.File]::ReadAllText($RequestPath) | ConvertFrom-Json
    $site = $request.site
    $target = $site.iis.site
    if ($site.iis.application -ne '/') { $target += $site.iis.application }
    $prefix = 'rumable-' + $site.id + '-'
    $vpath = '/__rumable__'
    $configPath = Join-Path $env:SystemRoot 'System32\inetsrv\config\applicationHost.config'
    Add-Type -Path (Join-Path $env:SystemRoot 'System32\inetsrv\Microsoft.Web.Administration.dll')
    $manager = New-Object Microsoft.Web.Administration.ServerManager
    $webSite = $manager.Sites[$site.iis.site]
    if ($null -eq $webSite) { throw 'IIS site not found.' }
    $application = $webSite.Applications[$site.iis.application]
    if ($null -eq $application) { throw 'IIS application not found.' }
    $config = $manager.GetApplicationHostConfiguration()
    $rumWanted = $null -eq $site.rum.enabled -or [bool]$site.rum.enabled
    $rewrite = $null; $rules = @(); $preconditions = @()
    try {
        $rewrite = $config.GetSection('system.webServer/rewrite/outboundRules', $target)
        $rules = $rewrite.GetCollection()
        $preconditions = $rewrite.GetCollection('preConditions')
    } catch { if ($rumWanted) { throw } }
    if ($rumWanted -and $null -ne $rewrite -and $rewrite.GetAttributeValue('rewriteBeforeCache')) { throw 'rewriteBeforeCache=true conflicts with streaming/chunked responses; review it manually.' }
    $inboundSection=$null; $allowedSection=$null; $inboundRules=@(); $allowedVariables=@()
    try {
        $inboundSection=$config.GetSection('system.webServer/rewrite/rules',$target); $inboundRules=$inboundSection.GetCollection()
        $allowedSection=$config.GetSection('system.webServer/rewrite/allowedServerVariables',$target); $allowedVariables=$allowedSection.GetCollection()
    } catch { if ($rumWanted) { throw } }
    $compression = $config.GetSection('system.webServer/urlCompression', $target)
    . (Join-Path $request.root 'otel-iis.ps1')
    . (Join-Path $request.root 'iis-rum.ps1')
    . (Join-Path $request.root 'iis-acl.ps1')

    function Read-Snapshot {
        # Capture owned XML and exact local attributes, including inheritance.
        [xml]$xml = [IO.File]::ReadAllText($configPath)
        # Use DOM methods: PowerShell's XML adapter turns empty elements into
        # strings and lets attributes (notably name) hide DOM properties.
        $location = $xml.SelectNodes('/configuration/location') | Where-Object { $_.GetAttribute('path') -ieq $target } | Select-Object -First 1
        $ownedRules = @()
        $ownedPreconditions = @()
        if ($null -ne $location) {
            $ownedRules = @($location.SelectNodes('system.webServer/rewrite/outboundRules/rule') | Where-Object { $_.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal) } | ForEach-Object { $_.get_OuterXml() })
            $ownedPreconditions = @($location.SelectNodes('system.webServer/rewrite/outboundRules/preConditions/preCondition') | Where-Object { $_.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal) } | ForEach-Object { $_.get_OuterXml() })
        }
        $localCompression = $null
        if ($null -ne $location) { $localCompression = $location.SelectSingleNode('system.webServer/urlCompression') }
        $present = $null -ne $localCompression -and $localCompression.HasAttribute('doStaticCompression')
        $value = $null
        if ($present) { $value = $localCompression.GetAttribute('doStaticCompression') }
        $dynamicPresent = $null -ne $localCompression -and $localCompression.HasAttribute('doDynamicCompression')
        $dynamicValue = $null
        if ($dynamicPresent) { $dynamicValue = $localCompression.GetAttribute('doDynamicCompression') }
        $xmlSite = $xml.SelectNodes('/configuration/system.applicationHost/sites/site') | Where-Object { $_.GetAttribute('name') -ieq $site.iis.site } | Select-Object -First 1
        $xmlApp = $null; $virtual = $null
        if ($null -ne $xmlSite) { $xmlApp = $xmlSite.SelectNodes('application') | Where-Object { $_.GetAttribute('path') -ieq $site.iis.application } | Select-Object -First 1 }
        if ($null -ne $xmlApp) { $virtual = $xmlApp.SelectNodes('virtualDirectory') | Where-Object { $_.GetAttribute('path') -ieq $vpath } | Select-Object -First 1 }
        $virtualXml = $null
        if ($null -ne $virtual) { $virtualXml = $virtual.get_OuterXml() }
        $snapshot = @{ rules=$ownedRules; preconditions=$ownedPreconditions; virtualDirectory=$virtualXml; compressionPresent=[bool]$present; compressionValue=$value; dynamicCompressionPresent=[bool]$dynamicPresent; dynamicCompressionValue=$dynamicValue }
        $snapshot.cacheGuard=Read-CacheGuard $location
        if ($null -ne $site.otel) { $snapshot.otel = Read-OTelSnapshot }
        return $snapshot
    }

    function Remove-Owned($collection) {
        for ($i=$collection.Count-1; $i -ge 0; $i--) {
            if (([string]$collection[$i].GetAttributeValue('name')).StartsWith($prefix, [StringComparison]::Ordinal)) { $collection.RemoveAt($i) }
        }
    }

    function Import-Element($element, $xmlNode) {
        foreach ($attribute in $xmlNode.Attributes) {
            $value = [string]$attribute.Value
            if (@('enabled', 'ignoreCase', 'negate', 'stopProcessing', 'trackAllCaptures', 'replace') -contains $attribute.Name) { $value = [bool]::Parse($value) }
            elseif ($attribute.Name -eq 'occurrences') { $value = [int]::Parse($value) }
            $element.SetAttributeValue($attribute.Name, $value)
        }
        foreach ($child in $xmlNode.ChildNodes) {
            if ($child.NodeType -ne [Xml.XmlNodeType]::Element) { continue }
            if (@('add','set') -ccontains $child.get_Name()) {
                $collection = $element.GetCollection()
                $new = $collection.CreateElement($child.get_Name())
                Import-Element $new $child
                [void]$collection.Add($new)
            } else {
                Import-Element ($element.GetChildElement($child.get_Name())) $child
            }
        }
    }

    function Restore-Compression($snapshot, $current) {
        # An OTel-only pool change must work even when URL compression is locked.
        foreach ($setting in @(@('doStaticCompression','compressionPresent','compressionValue'), @('doDynamicCompression','dynamicCompressionPresent','dynamicCompressionValue'))) {
            $attribute=$setting[0]; $present=$setting[1]; $value=$setting[2]
            # Old journals did not own dynamic compression. Never infer their
            # missing field as false or delete an administrator's local value.
            if ($null -eq $snapshot.$present) { continue }
            if ($snapshot.$present -eq $current.$present -and $snapshot.$value -ceq $current.$value) { continue }
            if ($snapshot.$present) { $compression.SetAttributeValue($attribute, [bool]::Parse($snapshot.$value)) }
            else { $compression.GetAttribute($attribute).Delete() }
        }
    }

    function Restore-Nodes($snapshot) {
        $current = Read-Snapshot
        Remove-Owned $rules
        Remove-Owned $preconditions
        foreach ($text in @($snapshot.rules)) {
            if ([string]::IsNullOrEmpty($text)) { continue }
            [xml]$doc = $text
            if ($doc.DocumentElement.get_Name() -cne 'rule' -or -not $doc.DocumentElement.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal)) { throw 'Invalid owned rule in snapshot.' }
            $element = $rules.CreateElement('rule')
            Import-Element $element $doc.DocumentElement
            [void]$rules.Add($element)
        }
        foreach ($text in @($snapshot.preconditions)) {
            if ([string]::IsNullOrEmpty($text)) { continue }
            [xml]$doc = $text
            if ($doc.DocumentElement.get_Name() -cne 'preCondition' -or -not $doc.DocumentElement.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal)) { throw 'Invalid owned precondition in snapshot.' }
            $element = $preconditions.CreateElement('preCondition')
            Import-Element $element $doc.DocumentElement
            [void]$preconditions.Add($element)
        }
        $virtual = $application.VirtualDirectories[$vpath]
        # Keep managed assets reachable for already cached HTML, even when
        # rolling back to the original disabled snapshot without a mapping.
        if ($null -ne $virtual -and $snapshot.virtualDirectory -cne $current.virtualDirectory -and ($null -ne $snapshot.virtualDirectory -or $request.removeMapping)) {
            if ($request.removeMapping -and $virtual.PhysicalPath -ne (Join-Path $request.root 'public')) { throw 'Uninstall refuses a virtual directory owned by another deployment.' }
            [void]$application.VirtualDirectories.Remove($virtual)
        }
        if ($null -ne $snapshot.virtualDirectory -and $snapshot.virtualDirectory -cne $current.virtualDirectory) {
            [xml]$doc = $snapshot.virtualDirectory
            if ($doc.DocumentElement.GetAttribute('path') -ne $vpath -or $doc.DocumentElement.GetAttribute('physicalPath') -ne (Join-Path $request.root 'public')) { throw 'Invalid virtual directory in snapshot.' }
            $virtual = $application.VirtualDirectories.Add($vpath, $doc.DocumentElement.GetAttribute('physicalPath'))
            foreach ($attribute in $doc.DocumentElement.Attributes) {
                if ($attribute.Name -ne 'path' -and $attribute.Name -ne 'physicalPath') { $virtual.SetAttributeValue($attribute.Name, [string]$attribute.Value) }
            }
        }
        Restore-CacheGuard $snapshot.cacheGuard $current.cacheGuard
        Restore-Compression $snapshot $current
        if ($null -ne $site.otel) { Restore-OTel $snapshot.otel }
    }

    function Assert-Snapshot($expected, $actual) {
        if (-not (Test-CacheGuard $expected.cacheGuard $actual.cacheGuard)) { throw 'IIS request revalidation changed before commit; refusing overwrite.' }
        foreach ($key in @('compressionPresent','compressionValue','virtualDirectory')) {
            if ($expected.$key -cne $actual.$key) { throw 'IIS configuration changed before commit; refusing overwrite.' }
        }
        if ($null -ne $expected.dynamicCompressionPresent) {
            foreach ($key in @('dynamicCompressionPresent','dynamicCompressionValue')) {
                if ($expected.$key -cne $actual.$key) { throw 'IIS dynamic compression changed before commit; refusing overwrite.' }
            }
        }
        foreach ($key in @('rules','preconditions')) {
            if ((@($expected.$key) | ConvertTo-Json -Compress) -cne (@($actual.$key) | ConvertTo-Json -Compress)) { throw 'IIS owned rules changed before commit; refusing overwrite.' }
        }
        if (($null -eq $expected.otel) -ne ($null -eq $actual.otel)) { throw 'IIS OTel ownership changed before commit.' }
        if ($null -eq $expected.otel) { return }
        foreach ($key in @('pool','runtime','pipeline','x86','identity','user')) {
            if ($expected.otel.$key -cne $actual.otel.$key) { throw 'IIS OTel pool metadata changed before commit; refusing overwrite.' }
        }
        if ((@($expected.otel.applications) | ConvertTo-Json -Compress) -cne (@($actual.otel.applications) | ConvertTo-Json -Compress)) { throw 'IIS OTel pool membership changed before commit.' }
        $values = $expected.otel.environment
        if ($values -is [Collections.IDictionary]) { $names = @($values.Keys) } else { $names = @($values.PSObject.Properties.Name) }
        if ($names.Count -ne $actual.otel.environment.Count) { throw 'IIS OTel environment changed before commit.' }
        foreach ($key in $names) {
            if (-not $actual.otel.environment.ContainsKey($key) -or $values.$key -cne $actual.otel.environment.$key) { throw 'IIS OTel environment changed before commit.' }
        }
    }

    $before = Read-Snapshot
    if (@('apply','restore','activate') -contains $request.action) {
        if ($null -eq $request.expected) { throw 'Expected IIS snapshot is required before changing configuration.' }
        Assert-Snapshot $request.expected $before
    }
    function Assert-LocalOwned($collection, $texts) {
        $localNames = @($texts | ForEach-Object { ([xml]$_).DocumentElement.GetAttribute('name') })
        foreach ($entry in $collection) {
            $entryName = [string]$entry.GetAttributeValue('name')
            if ($entryName.StartsWith($prefix, [StringComparison]::Ordinal) -and -not ($localNames -ccontains $entryName)) { throw 'Inherited Rumable rule name conflict; use a unique site ID and review parent configuration.' }
        }
    }
    Assert-LocalOwned $inboundRules $before.cacheGuard.rules
    Assert-LocalOwned $rules $before.rules
    Assert-LocalOwned $preconditions $before.preconditions
    if ($request.action -eq 'dependency-check') {
        $dependency = [string]$request.dependencyPath
        if (-not [IO.Path]::IsPathRooted($dependency)) { throw 'Expected absolute dependency path.' }
        foreach ($candidate in $manager.ApplicationPools) {
            foreach ($entry in $candidate.GetCollection('environmentVariables')) {
                if (([string]$entry.GetAttributeValue('value')).IndexOf($dependency,[StringComparison]::OrdinalIgnoreCase) -ge 0) { throw ('OTel dependency is still referenced by application pool ' + $candidate.Name + '; refusing removal.') }
            }
        }
        $globalModules = @($config.GetSection('system.webServer/globalModules').GetCollection() | ForEach-Object { [string]$_.GetAttributeValue('name') })
        if ($globalModules -contains 'AspNetCoreModule' -or $globalModules -contains 'AspNetCoreModuleV2') {
            foreach ($candidateSite in $manager.Sites) {
                foreach ($candidateApp in $candidateSite.Applications) {
                    $candidateConfig = $manager.GetWebConfiguration($candidateSite.Name,$candidateApp.Path)
                    foreach ($entry in $candidateConfig.GetSection('system.webServer/aspNetCore').GetCollection('environmentVariables')) {
                        if (([string]$entry.GetAttributeValue('value')).IndexOf($dependency,[StringComparison]::OrdinalIgnoreCase) -ge 0) { throw 'OTel dependency is still referenced by ASP.NET Core application environment; refusing removal.' }
                    }
                }
            }
        }
        foreach ($value in [Environment]::GetEnvironmentVariables('Machine').Values) {
            if (([string]$value).IndexOf($dependency,[StringComparison]::OrdinalIgnoreCase) -ge 0) { throw 'OTel dependency is still referenced by machine environment; refusing removal.' }
        }
        foreach ($service in @('WAS','W3SVC')) {
            $servicePath = 'HKLM:\SYSTEM\CurrentControlSet\Services\' + $service
            if (-not (Test-Path -LiteralPath $servicePath)) { continue }
            foreach ($entry in @((Get-ItemProperty -LiteralPath $servicePath).Environment)) {
                if (([string]$entry).IndexOf($dependency,[StringComparison]::OrdinalIgnoreCase) -ge 0) { throw 'OTel dependency is still referenced by WAS/W3SVC; refusing removal.' }
            }
        }
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'install-check') {
        if ($null -eq $site.otel) { throw 'Configure OTel before installing the Agent.' }
        $pool = Get-OTelPool
        if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { throw 'IIS OTel requires Windows x64.' }
        $users = @($manager.Sites | ForEach-Object { $_.Applications } | Where-Object { $_.ApplicationPoolName -ieq $pool.Name })
        if ($users.Count -ne 1) { throw 'IIS OTel requires a dedicated application pool.' }
        Assert-OTelExternalEnvironment
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'check') {
        if ($request.checkingEnabled -and $rumWanted) { Get-RumAssetReaders | Out-Null; Assert-CacheGuardWritable }
        if ($rumWanted -and ($compression.IsLocked -or $rewrite.IsLocked)) { throw 'IIS rewrite/compression configuration is locked.' }
        if ($request.checkingEnabled -and $rumWanted -and ($compression.GetAttributeValue('doStaticCompression') -or $compression.GetAttributeValue('doDynamicCompression')) -and -not $site.iis.allowCompressionChanges) { throw 'Static or dynamic compression is enabled; set allowCompressionChanges after reviewing its impact.' }
        if ($request.checkingEnabled -and $null -ne $site.otel -and $site.otel.enabled) { Assert-OTelPool -SavedCredentials:([bool]$request.restoringOTel) }
        $virtual = $application.VirtualDirectories[$vpath]
        $expectedPath = Join-Path $request.root 'public'
        if ($rumWanted -and $null -ne $virtual -and $virtual.PhysicalPath -ne $expectedPath) { throw 'Reserved /__rumable__ virtual directory is already owned by another deployment.' }
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'prepare-rum-rollback') {
        $request.snapshot.cacheGuard=New-EnabledCacheGuard $request.snapshot.cacheGuard
        $request.snapshot | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'inspect') {
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'recover-pending-check') {
        Assert-PendingRumSnapshot $before
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'activate') {
        # Go persists the committed snapshot before this separate process can
        # recycle a pool, so recycle failure/timeout remains recoverable.
        Recycle-OTelPool $request.previous.otel
        Read-Snapshot | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'restore') {
        Restore-Nodes $request.snapshot
        $manager.CommitChanges()
        Read-Snapshot | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'apply') {
        if ($request.enabled) { Set-RumPublicACL; Assert-CacheGuardWritable }
        Remove-Owned $rules
        Remove-Owned $preconditions
        if ($request.enabled) {
            Restore-CacheGuard (New-EnabledCacheGuard $before.cacheGuard) $before.cacheGuard
            $compression.SetAttributeValue('doStaticCompression', $false)
            $compression.SetAttributeValue('doDynamicCompression', $false)
            $nodes = Get-RumNodes
            foreach ($entry in @(@('preCondition', $preconditions, $nodes.preconditions), @('rule', $rules, $nodes.rules))) {
                foreach ($text in $entry[2]) {
                    [xml]$doc = $text
                    $element = $entry[1].CreateElement($entry[0])
                    Import-Element $element $doc.DocumentElement
                    [void]$entry[1].Add($element)
                }
            }
        } else {
            Restore-CacheGuard $request.original.cacheGuard $before.cacheGuard
            Restore-Compression $request.original $before
        }
        if ($request.enabled -and $null -eq $application.VirtualDirectories[$vpath]) { [void]$application.VirtualDirectories.Add($vpath, (Join-Path $request.root 'public')) }
        Apply-OTel
        $manager.CommitChanges()
        Read-Snapshot | ConvertTo-Json -Depth 30 -Compress
    } else { throw 'Unknown bridge action.' }
    $manager.Dispose()
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
