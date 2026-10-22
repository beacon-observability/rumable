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
            'set/replace'='true'; 'set/value'=''; 'match/negate'='false'; 'add/negate'='false'; 'add/ignoreCase'='true'; 'action/value'=''
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
    if ($null -ne $wanted.dynamicCompressionPresent) {
        foreach ($key in @('dynamicCompressionPresent','dynamicCompressionValue')) { if ($wanted.$key -cne $live.$key) { return $false } }
    }
    if (-not (Test-CacheGuard $wanted.cacheGuard $live.cacheGuard)) { return $false }
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
        if ($null -ne $request.previous.dynamicCompressionPresent) {
            $wanted.dynamicCompressionPresent=$request.original.dynamicCompressionPresent
            $wanted.dynamicCompressionValue=$request.original.dynamicCompressionValue
        }
        if ($null -ne $request.previous.cacheGuard) { $wanted.cacheGuard=$request.original.cacheGuard }
        if ($request.enabled) {
            if ($null -ne $request.previous.cacheGuard) { $wanted.cacheGuard=New-EnabledCacheGuard $request.previous.cacheGuard }
            $nodes = Get-RumNodes -LegacyNullExclude:$legacy
            $wanted.rules=$nodes.rules; $wanted.preconditions=$nodes.preconditions
            $wanted.compressionPresent=$true; $wanted.compressionValue='false'
            if ($null -ne $request.previous.dynamicCompressionPresent) {
                $wanted.dynamicCompressionPresent=$true; $wanted.dynamicCompressionValue='false'
            }
            if ($null -eq $wanted.virtualDirectory) {
                $doc = New-Object Xml.XmlDocument
                $virtual = $doc.CreateElement('virtualDirectory'); $virtual.SetAttribute('path', $vpath); $virtual.SetAttribute('physicalPath', (Join-Path $request.root 'public'))
                $wanted.virtualDirectory=$virtual.OuterXml
            }
        }
        if (Test-OwnedSnapshot $wanted $live) { return }
        # A preceding rollback may already have restored rules/compression but
        # retained the first public mapping before its response was interrupted.
        if ($null -ne $request.previous.cacheGuard) { $wanted.cacheGuard=$request.previous.cacheGuard }
        $wanted.rules=$request.previous.rules; $wanted.preconditions=$request.previous.preconditions
        $wanted.compressionPresent=$request.previous.compressionPresent; $wanted.compressionValue=$request.previous.compressionValue
        if ($null -ne $request.previous.dynamicCompressionPresent) {
            $wanted.dynamicCompressionPresent=$request.previous.dynamicCompressionPresent
            $wanted.dynamicCompressionValue=$request.previous.dynamicCompressionValue
        }
        if (Test-OwnedSnapshot $wanted $live) { return }
    }
    throw 'IIS state differs from the exact pending RUM operation; manual review required before recovery.'
}

# Conditional GET protection is a separate owned inbound rule. HTML navigation
# requests must reach the handler without validators from an older representation.
$cacheHeaderNames = @('HTTP_IF_NONE_MATCH','HTTP_IF_MODIFIED_SINCE')
function Get-CacheGuardRule {
    $doc=New-Object Xml.XmlDocument
    $rule=$doc.CreateElement('rule'); $rule.SetAttribute('name',($prefix+'revalidate'))
    $match=$doc.CreateElement('match'); $match.SetAttribute('url','.*'); [void]$rule.AppendChild($match)
    $conditions=$doc.CreateElement('conditions')
    # Reuse the exact path predicates, including the reserved resource exclusion.
    [xml]$outbound=(Get-RumNodes).rules[0]
    foreach ($condition in $outbound.DocumentElement.SelectNodes('conditions/add')) { [void]$conditions.AppendChild($doc.ImportNode($condition,$true)) }
    foreach ($check in @(@('{REQUEST_METHOD}','^GET$'),@('{HTTP_ACCEPT}','(?:^|,)\s*text/html(?:\s*;|\s*,|\s*$)'))) {
        $add=$doc.CreateElement('add'); $add.SetAttribute('input',$check[0]); $add.SetAttribute('pattern',$check[1]); [void]$conditions.AppendChild($add)
    }
    [void]$rule.AppendChild($conditions)
    $variables=$doc.CreateElement('serverVariables')
    foreach ($name in $cacheHeaderNames) {
        $set=$doc.CreateElement('set'); $set.SetAttribute('name',$name); $set.SetAttribute('value',''); [void]$variables.AppendChild($set)
    }
    [void]$rule.AppendChild($variables)
    $action=$doc.CreateElement('action'); $action.SetAttribute('type','None'); [void]$rule.AppendChild($action)
    return $rule.OuterXml
}

function Read-CacheGuard($location) {
    $owned=@(); $local=@(); $effective=@(); $index=-1
    if ($null -ne $location) {
        $all=@($location.SelectNodes('system.webServer/rewrite/rules/rule'))
        for ($i=0;$i -lt $all.Count;$i++) {
            if ($all[$i].GetAttribute('name').StartsWith($prefix,[StringComparison]::Ordinal)) {
                if ($index -lt 0) { $index=$i }
                $owned+=$all[$i].get_OuterXml()
            }
        }
        $local=@($location.SelectNodes('system.webServer/rewrite/allowedServerVariables/add') | ForEach-Object { $_.GetAttribute('name') } | Where-Object { $cacheHeaderNames -ccontains $_ } | Sort-Object)
    }
    $effective=@($allowedVariables | Where-Object { $null -ne $_ } | ForEach-Object { [string]$_.GetAttributeValue('name') } | Where-Object { $cacheHeaderNames -ccontains $_ } | Sort-Object)
    return @{rules=$owned;ruleIndex=$index;permissions=$local;effectivePermissions=$effective}
}

function New-EnabledCacheGuard($baseline) {
    $local=@($baseline.permissions | Where-Object { $null -ne $_ })
    foreach ($name in $cacheHeaderNames) { if (@($baseline.effectivePermissions) -cnotcontains $name) { $local+=$name } }
    return @{rules=@((Get-CacheGuardRule));ruleIndex=0;permissions=@($local | Sort-Object -Unique);effectivePermissions=@($cacheHeaderNames | Sort-Object)}
}

function Test-CacheGuard($wanted,$live) {
    if ($null -eq $wanted) { return $null -eq $live -or @($live.rules).Count -eq 0 }
    if ($null -eq $live -or $wanted.ruleIndex -ne $live.ruleIndex) { return $false }
    foreach ($key in @('permissions','effectivePermissions')) {
        if ((ConvertTo-Json -InputObject @($wanted.$key) -Compress) -cne (ConvertTo-Json -InputObject @($live.$key) -Compress)) { return $false }
    }
    $a=@($wanted.rules);$b=@($live.rules)
    if ($a.Count -ne $b.Count) { return $false }
    for ($i=0;$i -lt $a.Count;$i++) { if ((ConvertTo-CanonicalOwnedXml $a[$i]) -cne (ConvertTo-CanonicalOwnedXml $b[$i])) { return $false } }
    return $true
}

function Assert-CacheGuardWritable {
    if ($null -eq $inboundSection -or $null -eq $allowedSection -or $inboundSection.IsLocked -or $allowedSection.IsLocked) { throw 'IIS request revalidation configuration is locked or unavailable.' }
    [xml]$xml=[IO.File]::ReadAllText($configPath)
    $location=$xml.SelectNodes('/configuration/location') | Where-Object { $_.GetAttribute('path') -ieq $target } | Select-Object -First 1
    if ($null -ne $location) {
        foreach ($entry in $location.SelectNodes('system.webServer/rewrite/allowedServerVariables/remove')) {
            if ($cacheHeaderNames -contains $entry.GetAttribute('name')) { throw 'A local server-variable removal conflicts with RUM revalidation; review allowedServerVariables before enabling.' }
        }
    }
}

function Restore-CacheGuard($wanted,$current) {
    if ($null -eq $wanted -or (Test-CacheGuard $wanted $current)) { return }
    if (@($wanted.rules).Count -gt 1 -or (@($wanted.rules).Count -eq 1 -and $wanted.ruleIndex -ne 0)) { throw 'Unexpected owned inbound rule position; manual review required.' }
    foreach ($text in @($wanted.rules)) {
        [xml]$doc=$text
        if ($doc.DocumentElement.get_Name() -cne 'rule' -or $doc.DocumentElement.GetAttribute('name') -cne ($prefix+'revalidate')) { throw 'Invalid owned inbound rule in snapshot.' }
    }
    # Only two locally added permissions are owned. Inherited and pre-existing
    # entries remain intact; no collection clear or machine-wide setting changes.
    foreach ($name in $cacheHeaderNames) {
        $has=@($current.permissions) -ccontains $name; $want=@($wanted.permissions) -ccontains $name
        if ($has -eq $want) { continue }
        if ($want) {
            $entry=$allowedVariables.CreateElement('add');$entry.SetAttributeValue('name',$name);[void]$allowedVariables.Add($entry)
        } else {
            for ($i=$allowedVariables.Count-1;$i -ge 0;$i--) { if ($allowedVariables[$i].GetAttributeValue('name') -ceq $name) { [void]$allowedVariables.RemoveAt($i) } }
        }
    }
    Remove-Owned $inboundRules
    # Keep inherited rules in front, and run before local business rewrites.
    [xml]$xml=[IO.File]::ReadAllText($configPath)
    $location=$xml.SelectNodes('/configuration/location') | Where-Object { $_.GetAttribute('path') -ieq $target } | Select-Object -First 1
    $names=@(); if ($null -ne $location) { $names=@($location.SelectNodes('system.webServer/rewrite/rules/rule') | ForEach-Object { $_.GetAttribute('name') }) }
    $at=$inboundRules.Count
    for ($i=0;$i -lt $inboundRules.Count;$i++) { if ($names -ccontains $inboundRules[$i].GetAttributeValue('name')) { $at=$i;break } }
    foreach ($text in @($wanted.rules)) {
        [xml]$doc=$text;$element=$inboundRules.CreateElement('rule');Import-Element $element $doc.DocumentElement
        [void]$inboundRules.AddAt($at,$element);$at++
    }
}
