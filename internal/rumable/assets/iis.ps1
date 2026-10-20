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
    $compression = $config.GetSection('system.webServer/urlCompression', $target)
    . (Join-Path $request.root 'otel-iis.ps1')

    function Read-Snapshot {
        # Capture only owned XML nodes and the exact local compression attribute.
        [xml]$xml = [IO.File]::ReadAllText($configPath)
        $location = @($xml.configuration.location | Where-Object { $_.path -ieq $target }) | Select-Object -First 1
        $ownedRules = @()
        $ownedPreconditions = @()
        if ($null -ne $location) {
            $ownedRules = @($location.'system.webServer'.rewrite.outboundRules.rule | Where-Object { $null -ne $_ -and $_.name.StartsWith($prefix, [StringComparison]::Ordinal) } | ForEach-Object { $_.OuterXml })
            $ownedPreconditions = @($location.'system.webServer'.rewrite.outboundRules.preConditions.preCondition | Where-Object { $null -ne $_ -and $_.name.StartsWith($prefix, [StringComparison]::Ordinal) } | ForEach-Object { $_.OuterXml })
        }
        $localCompression = $null
        if ($null -ne $location) { $localCompression = $location.'system.webServer'.urlCompression }
        $present = $null -ne $localCompression -and $localCompression.HasAttribute('doStaticCompression')
        $value = $null
        if ($present) { $value = $localCompression.GetAttribute('doStaticCompression') }
        $xmlSite = @($xml.configuration.'system.applicationHost'.sites.site | Where-Object { $_.name -ieq $site.iis.site }) | Select-Object -First 1
        $xmlApp = @($xmlSite.application | Where-Object { $_.path -ieq $site.iis.application }) | Select-Object -First 1
        $virtual = @($xmlApp.virtualDirectory | Where-Object { $_.path -ieq $vpath }) | Select-Object -First 1
        $virtualXml = $null
        if ($null -ne $virtual) { $virtualXml = $virtual.OuterXml }
        $snapshot = @{ rules=$ownedRules; preconditions=$ownedPreconditions; virtualDirectory=$virtualXml; compressionPresent=[bool]$present; compressionValue=$value }
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
            if (@('enabled', 'ignoreCase', 'negate', 'stopProcessing', 'trackAllCaptures') -contains $attribute.Name) { $value = [bool]::Parse($value) }
            elseif ($attribute.Name -eq 'occurrences') { $value = [int]::Parse($value) }
            $element.SetAttributeValue($attribute.Name, $value)
        }
        foreach ($child in $xmlNode.ChildNodes) {
            if ($child.NodeType -ne [Xml.XmlNodeType]::Element) { continue }
            if ($child.Name -eq 'add') {
                $collection = $element.GetCollection()
                $new = $collection.CreateElement('add')
                Import-Element $new $child
                $collection.Add($new)
            } else {
                Import-Element ($element.GetChildElement($child.Name)) $child
            }
        }
    }

    function Restore-Compression($snapshot, $current) {
        # An OTel-only pool change must work even when URL compression is locked.
        if ($snapshot.compressionPresent -eq $current.compressionPresent -and $snapshot.compressionValue -ceq $current.compressionValue) { return }
        if ($snapshot.compressionPresent) {
            $compression.SetAttributeValue('doStaticCompression', [bool]::Parse($snapshot.compressionValue))
        } else {
            $compression.GetAttribute('doStaticCompression').Delete()
        }
    }

    function Restore-Nodes($snapshot) {
        $current = Read-Snapshot
        Remove-Owned $rules
        Remove-Owned $preconditions
        foreach ($text in @($snapshot.rules)) {
            if ([string]::IsNullOrEmpty($text)) { continue }
            [xml]$doc = $text
            if ($doc.DocumentElement.Name -ne 'rule' -or -not $doc.DocumentElement.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal)) { throw 'Invalid owned rule in snapshot.' }
            $element = $rules.CreateElement('rule')
            Import-Element $element $doc.DocumentElement
            $rules.Add($element)
        }
        foreach ($text in @($snapshot.preconditions)) {
            if ([string]::IsNullOrEmpty($text)) { continue }
            [xml]$doc = $text
            if ($doc.DocumentElement.Name -ne 'preCondition' -or -not $doc.DocumentElement.GetAttribute('name').StartsWith($prefix, [StringComparison]::Ordinal)) { throw 'Invalid owned precondition in snapshot.' }
            $element = $preconditions.CreateElement('preCondition')
            Import-Element $element $doc.DocumentElement
            $preconditions.Add($element)
        }
        $virtual = $application.VirtualDirectories[$vpath]
        # Keep managed assets reachable for already cached HTML, even when
        # rolling back to the original disabled snapshot without a mapping.
        if ($null -ne $virtual -and $snapshot.virtualDirectory -cne $current.virtualDirectory -and ($null -ne $snapshot.virtualDirectory -or $request.removeMapping)) {
            if ($request.removeMapping -and $virtual.PhysicalPath -ne (Join-Path $request.root 'public')) { throw 'Uninstall refuses a virtual directory owned by another deployment.' }
            $application.VirtualDirectories.Remove($virtual)
        }
        if ($null -ne $snapshot.virtualDirectory -and $snapshot.virtualDirectory -cne $current.virtualDirectory) {
            [xml]$doc = $snapshot.virtualDirectory
            if ($doc.DocumentElement.GetAttribute('path') -ne $vpath -or $doc.DocumentElement.GetAttribute('physicalPath') -ne (Join-Path $request.root 'public')) { throw 'Invalid virtual directory in snapshot.' }
            $virtual = $application.VirtualDirectories.Add($vpath, $doc.DocumentElement.GetAttribute('physicalPath'))
            foreach ($attribute in $doc.DocumentElement.Attributes) {
                if ($attribute.Name -ne 'path' -and $attribute.Name -ne 'physicalPath') { $virtual.SetAttributeValue($attribute.Name, [string]$attribute.Value) }
            }
        }
        Restore-Compression $snapshot $current
        if ($null -ne $site.otel) { Restore-OTel $snapshot.otel }
    }

    function Assert-Snapshot($expected, $actual) {
        foreach ($key in @('compressionPresent','compressionValue','virtualDirectory')) {
            if ($expected.$key -cne $actual.$key) { throw 'IIS configuration changed before commit; refusing overwrite.' }
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
        if ($rumWanted -and ($compression.IsLocked -or $rewrite.IsLocked)) { throw 'IIS rewrite/compression configuration is locked.' }
        if ($rumWanted -and $compression.GetAttributeValue('doStaticCompression') -and -not $site.iis.allowCompressionChanges) { throw 'Static compression is enabled; set allowCompressionChanges after reviewing its impact.' }
        if ($request.checkingEnabled -and $null -ne $site.otel -and $site.otel.enabled) { Assert-OTelPool -SavedCredentials:([bool]$request.restoringOTel) }
        $virtual = $application.VirtualDirectories[$vpath]
        $expectedPath = Join-Path $request.root 'public'
        if ($rumWanted -and $null -ne $virtual -and $virtual.PhysicalPath -ne $expectedPath) { throw 'Reserved /__rumable__ virtual directory is already owned by another deployment.' }
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'inspect') {
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
        Remove-Owned $rules
        Remove-Owned $preconditions
        if ($request.enabled) {
            $compression.SetAttributeValue('doStaticCompression', $false)
            $pre = $preconditions.CreateElement('preCondition')
            $pre.SetAttributeValue('name', ($prefix + 'html'))
            $checks = @(
                @('{RESPONSE_CONTENT_TYPE}', '^text/html(?:;|$)', $false),
                @('{RESPONSE_CONTENT_ENCODING}', '^$', $false),
                @('{RESPONSE_CONTENT_DISPOSITION}', 'attachment', $true),
                @('{REQUEST_METHOD}', '^GET$', $false)
            )
            foreach ($check in $checks) {
                $element = $pre.GetCollection().CreateElement('add')
                $element.SetAttributeValue('input', $check[0]); $element.SetAttributeValue('pattern', $check[1]); $element.SetAttributeValue('negate', [bool]$check[2])
                $pre.GetCollection().Add($element)
            }
            $preconditions.Add($pre)
            $include = '^(?:' + ((@($site.includePaths) | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')'
            $exclude = @($site.excludePaths) + @(([uri]$site.publicURL).AbsolutePath.TrimEnd('/') + '/__rumable__/')
            $excludePattern = '^(?:' + (($exclude | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')'
            foreach ($kind in @('inject', 'etag', 'modified')) {
                $rule = $rules.CreateElement('rule')
                $rule.SetAttributeValue('name', ($prefix + $kind))
                $rule.SetAttributeValue('preCondition', ($prefix + 'html'))
                $match = $rule.GetChildElement('match')
                if ($kind -eq 'inject') {
                    $match.SetAttributeValue('filterByTags', 'None'); $match.SetAttributeValue('pattern', '</head>'); $match.SetAttributeValue('occurrences', 1)
                    $value = '<script async data-rumable="' + $site.id + '" src="' + $request.script + '"></script></head>'
                } else {
                    $header = 'RESPONSE_ETag'; if ($kind -eq 'modified') { $header = 'RESPONSE_Last_Modified' }
                    $match.SetAttributeValue('serverVariable', $header); $match.SetAttributeValue('pattern', '.+')
                    $value = ''
                }
                $conditions = $rule.GetCollection('conditions')
                foreach ($condition in @(@($include, $false), @($excludePattern, $true))) {
                    $element = $conditions.CreateElement('add'); $element.SetAttributeValue('input', '{URL}'); $element.SetAttributeValue('pattern', $condition[0]); $element.SetAttributeValue('negate', [bool]$condition[1]); $element.SetAttributeValue('ignoreCase', $false); $conditions.Add($element)
                }
                $action = $rule.GetChildElement('action'); $action.SetAttributeValue('type', 'Rewrite'); $action.SetAttributeValue('value', $value)
                $rules.Add($rule)
            }
        } else {
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
