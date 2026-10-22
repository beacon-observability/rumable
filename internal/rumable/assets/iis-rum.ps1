# Pure rule construction, shared by apply and recovery validation. No IIS writes.
function Get-RumNodes([switch]$LegacyNullExclude) {
    $includePaths = @($site.includePaths | Where-Object { -not [string]::IsNullOrEmpty($_) })
    if ($includePaths.Count -eq 0) { throw 'RUM requires at least one nonempty include path.' }
    $include = '^(?:' + (($includePaths | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')'
    $exclude = @($site.excludePaths | Where-Object { -not [string]::IsNullOrEmpty($_) })
    $exclude += ([uri]$site.publicURL).AbsolutePath.TrimEnd('/') + '/__rumable__/'
    $excludePattern = '^(?:' + (($exclude | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')'
    # Only recognize the exact 0.1.4 null-slice bug when recovering old journals.
    if ($LegacyNullExclude -and $null -eq $site.excludePaths) { $excludePattern = '^(?:|' + [regex]::Escape($exclude[-1]) + ')' }
    $doc = New-Object Xml.XmlDocument
    $pre = $doc.CreateElement('preCondition')
    $pre.SetAttribute('name', ($prefix + 'html'))
    foreach ($check in @(
        @('{RESPONSE_CONTENT_TYPE}', '^text/html(?:;|$)', 'false'),
        @('{RESPONSE_CONTENT_ENCODING}', '^$', 'false'),
        @('{RESPONSE_CONTENT_DISPOSITION}', 'attachment', 'true'),
        @('{REQUEST_METHOD}', '^GET$', 'false')
    )) {
        $add = $doc.CreateElement('add')
        $add.SetAttribute('input', $check[0]); $add.SetAttribute('pattern', $check[1]); $add.SetAttribute('negate', $check[2])
        [void]$pre.AppendChild($add)
    }
    $result = @()
    foreach ($kind in @('inject', 'etag', 'modified')) {
        $rule = $doc.CreateElement('rule'); $rule.SetAttribute('name', ($prefix + $kind)); $rule.SetAttribute('preCondition', ($prefix + 'html'))
        $match = $doc.CreateElement('match')
        if ($kind -eq 'inject') {
            $match.SetAttribute('filterByTags', 'None'); $match.SetAttribute('pattern', '<head>'); $match.SetAttribute('occurrences', '1')
            $value = '<head><script async data-rumable="' + $site.id + '" src="' + $request.script + '"></script>'
            # Go builds both ordered synchronous tags from the saved SDK metadata.
            # Legacy requests retain their exact asynchronous rule for recovery.
            if ($request.injection) { $value = '<head>' + $request.injection }
        } else {
            $header = 'RESPONSE_ETag'; if ($kind -eq 'modified') { $header = 'RESPONSE_Last_Modified' }
            $match.SetAttribute('serverVariable', $header); $match.SetAttribute('pattern', '.+')
            $value = ''
        }
        [void]$rule.AppendChild($match)
        $conditions = $doc.CreateElement('conditions')
        foreach ($condition in @(@($include, 'false'), @($excludePattern, 'true'))) {
            $add = $doc.CreateElement('add'); $add.SetAttribute('input', '{URL}'); $add.SetAttribute('pattern', $condition[0]); $add.SetAttribute('negate', $condition[1]); $add.SetAttribute('ignoreCase', 'false')
            [void]$conditions.AppendChild($add)
        }
        [void]$rule.AppendChild($conditions)
        $action = $doc.CreateElement('action'); $action.SetAttribute('type', 'Rewrite'); $action.SetAttribute('value', $value)
        [void]$rule.AppendChild($action)
        $result += $rule.OuterXml
    }
    return @{rules=$result; preconditions=@($pre.OuterXml)}
}

function ConvertTo-CanonicalOwnedXml([string]$text) {
    # MWA omits schema-default attributes and changes their ordering. Ignore
    # only these known defaults; preserve every other attribute and child.
    [xml]$doc = $text
    function Canonical-Node($node) {
        $defaults = @{
            'rule/enabled'='true'; 'rule/patternSyntax'='ECMAScript'; 'rule/stopProcessing'='false'
            'match/ignoreCase'='true'; 'match/filterByTags'='None'; 'match/serverVariable'=''; 'match/occurrences'='0'
            'conditions/logicalGrouping'='MatchAll'; 'conditions/trackAllCaptures'='false'
            'preCondition/logicalGrouping'='MatchAll'; 'preCondition/patternSyntax'='ECMAScript'
            'add/negate'='false'; 'add/ignoreCase'='true'; 'action/value'=''
        }
        $nodeName = $node.get_Name()
        $attributes = @($node.Attributes | Where-Object { $defaults[($nodeName + '/' + $_.Name)] -cne $_.Value } | Sort-Object Name | ForEach-Object { @($_.Name, $_.Value) })
        $children = @($node.ChildNodes | Where-Object { $_.NodeType -eq [Xml.XmlNodeType]::Element } | ForEach-Object { Canonical-Node $_ })
        if (@($node.ChildNodes | Where-Object { $_.NodeType -notin @([Xml.XmlNodeType]::Element,[Xml.XmlNodeType]::Whitespace,[Xml.XmlNodeType]::SignificantWhitespace) }).Count) { throw 'Unexpected content in owned IIS XML.' }
        return ,@($nodeName, $attributes, $children)
    }
    return ConvertTo-Json -InputObject (Canonical-Node $doc.DocumentElement) -Depth 40 -Compress
}

function Test-OwnedSnapshot($wanted, $live) {
    foreach ($key in @('compressionPresent','compressionValue')) { if ($wanted.$key -cne $live.$key) { return $false } }
    if ($null -ne $live.otel -or $null -ne $wanted.otel) { return $false }
    foreach ($key in @('rules','preconditions','virtualDirectory')) {
        $a = @($wanted.$key | Where-Object { $null -ne $_ }); $b = @($live.$key | Where-Object { $null -ne $_ })
        if ($a.Count -ne $b.Count) { return $false }
        for ($i=0; $i -lt $a.Count; $i++) {
            if ((ConvertTo-CanonicalOwnedXml $a[$i]) -cne (ConvertTo-CanonicalOwnedXml $b[$i])) { return $false }
        }
    }
    return $true
}

function Assert-PendingRumSnapshot($live) {
    if ($null -ne $site.otel -or $null -ne $request.previous.otel -or $request.uninstall) { throw 'Unacknowledged native transaction requires manual review for OTel/uninstall.' }
    # Reconstruct only the RUM-only apply represented by the durable journal.
    foreach ($legacy in @($false,$true)) {
        $wanted = @{ rules=@(); preconditions=@(); virtualDirectory=$request.previous.virtualDirectory; compressionPresent=$request.original.compressionPresent; compressionValue=$request.original.compressionValue }
        if ($request.enabled) {
            $nodes = Get-RumNodes -LegacyNullExclude:$legacy
            $wanted.rules=$nodes.rules; $wanted.preconditions=$nodes.preconditions
            $wanted.compressionPresent=$true; $wanted.compressionValue='false'
            if ($null -eq $wanted.virtualDirectory) {
                $doc = New-Object Xml.XmlDocument
                $virtual = $doc.CreateElement('virtualDirectory'); $virtual.SetAttribute('path', $vpath); $virtual.SetAttribute('physicalPath', (Join-Path $request.root 'public'))
                $wanted.virtualDirectory=$virtual.OuterXml
            }
        }
        if (Test-OwnedSnapshot $wanted $live) { return }
        # A preceding rollback may already have restored rules/compression but
        # retained the first public mapping before its response was interrupted.
        $wanted.rules=$request.previous.rules; $wanted.preconditions=$request.previous.preconditions
        $wanted.compressionPresent=$request.previous.compressionPresent; $wanted.compressionValue=$request.previous.compressionValue
        if (Test-OwnedSnapshot $wanted $live) { return }
    }
    throw 'IIS state differs from the exact pending RUM operation; manual review required before recovery.'
}
