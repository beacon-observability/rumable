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
    $rewrite = $config.GetSection('system.webServer/rewrite/outboundRules', $target)
    if ($rewrite.GetAttributeValue('rewriteBeforeCache')) { throw 'rewriteBeforeCache=true conflicts with streaming/chunked responses; review it manually.' }
    $rules = $rewrite.GetCollection()
    $preconditions = $rewrite.GetCollection('preConditions')
    $compression = $config.GetSection('system.webServer/urlCompression', $target)

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
        return @{ rules=$ownedRules; preconditions=$ownedPreconditions; virtualDirectory=$virtualXml; compressionPresent=[bool]$present; compressionValue=$value }
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

    function Restore-Nodes($snapshot) {
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
        if ($null -ne $virtual -and $null -ne $snapshot.virtualDirectory) { $application.VirtualDirectories.Remove($virtual) }
        if ($null -ne $snapshot.virtualDirectory) {
            [xml]$doc = $snapshot.virtualDirectory
            if ($doc.DocumentElement.GetAttribute('path') -ne $vpath -or $doc.DocumentElement.GetAttribute('physicalPath') -ne (Join-Path $request.root 'public')) { throw 'Invalid virtual directory in snapshot.' }
            $virtual = $application.VirtualDirectories.Add($vpath, $doc.DocumentElement.GetAttribute('physicalPath'))
            foreach ($attribute in $doc.DocumentElement.Attributes) {
                if ($attribute.Name -ne 'path' -and $attribute.Name -ne 'physicalPath') { $virtual.SetAttributeValue($attribute.Name, [string]$attribute.Value) }
            }
        }
        if ($snapshot.compressionPresent) {
            $compression.SetAttributeValue('doStaticCompression', [bool]::Parse($snapshot.compressionValue))
        } else {
            $compression.GetAttribute('doStaticCompression').Delete()
        }
    }

    $before = Read-Snapshot
    function Assert-LocalOwned($collection, $texts) {
        $localNames = @($texts | ForEach-Object { ([xml]$_).DocumentElement.GetAttribute('name') })
        foreach ($entry in $collection) {
            $entryName = [string]$entry.GetAttributeValue('name')
            if ($entryName.StartsWith($prefix, [StringComparison]::Ordinal) -and -not ($localNames -ccontains $entryName)) { throw 'Inherited Rumable rule name conflict; use a unique site ID and review parent configuration.' }
        }
    }
    Assert-LocalOwned $rules $before.rules
    Assert-LocalOwned $preconditions $before.preconditions
    if ($request.action -eq 'check') {
        if ($compression.IsLocked -or $rewrite.IsLocked) { throw 'IIS rewrite/compression configuration is locked.' }
        if ($compression.GetAttributeValue('doStaticCompression') -and -not $site.iis.allowCompressionChanges) { throw 'Static compression is enabled; set allowCompressionChanges after reviewing its impact.' }
        $virtual = $application.VirtualDirectories[$vpath]
        $expectedPath = Join-Path $request.root 'public'
        if ($null -ne $virtual -and $virtual.PhysicalPath -ne $expectedPath) { throw 'Reserved /__rumable__ virtual directory is already owned by another deployment.' }
        $before | ConvertTo-Json -Depth 30 -Compress
    } elseif ($request.action -eq 'inspect') {
        $before | ConvertTo-Json -Depth 30 -Compress
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
            if ($request.original.compressionPresent) { $compression.SetAttributeValue('doStaticCompression', [bool]::Parse($request.original.compressionValue)) } else { $compression.GetAttribute('doStaticCompression').Delete() }
        }
        if ($null -eq $application.VirtualDirectories[$vpath]) { [void]$application.VirtualDirectories.Add($vpath, (Join-Path $request.root 'public')) }
        $manager.CommitChanges()
        Read-Snapshot | ConvertTo-Json -Depth 30 -Compress
    } else { throw 'Unknown bridge action.' }
    $manager.Dispose()
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
