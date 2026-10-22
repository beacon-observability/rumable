$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
function Assert-That($value,[string]$message) { if (-not $value) { throw $message } }
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'),[ref]$tokens,[ref]$errors)
foreach ($name in @('Import-Element','Remove-Owned','Assert-Snapshot')) {
    $node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name},$true)
    Invoke-Expression $node.Extent.Text
}
Add-Type @'
using System.Collections;
public class CacheElement {
    public Hashtable Values=new Hashtable(), Children=new Hashtable();
    public CacheCollection Collection=new CacheCollection();
    public void SetAttributeValue(string name,object value) { Values[name]=value; }
    public object GetAttributeValue(string name) { return Values[name]; }
    public CacheElement GetChildElement(string name) { if(!Children.ContainsKey(name)) Children[name]=new CacheElement(); return (CacheElement)Children[name]; }
    public CacheCollection GetCollection() { return Collection; }
}
public class CacheCollection : IEnumerable {
    public ArrayList Items=new ArrayList();
    public int Count { get {return Items.Count;} }
    public CacheElement this[int i] {get {return (CacheElement)Items[i];} }
    public IEnumerator GetEnumerator() {return Items.GetEnumerator();}
    public CacheElement CreateElement(string name) {return new CacheElement();}
    public CacheElement Add(CacheElement item) {Items.Add(item);return item;}
    public CacheElement AddAt(int index,CacheElement item) {Items.Insert(index,item);return item;}
    public void RemoveAt(int index) {Items.RemoveAt(index);}
}
'@
$configPath=[IO.Path]::GetTempFileName()
$target='Site/app';$prefix='rumable-test-';$vpath='/__rumable__'
$site=@{id='test';publicURL='http://localhost/app/';includePaths=@('/app/');excludePaths=@('/app/excluded/')}
$request=@{script='/app/__rumable__/test/init.js';root=[IO.Path]::GetTempPath()}
try {
    [xml]$rule=Get-CacheGuardRule
    Assert-That ($rule.DocumentElement.get_Name() -ceq 'rule' -and $rule.rule.action.type -ceq 'None') 'Inbound guard changed business URL'
    Assert-That (@($rule.rule.serverVariables.set).Count -eq 2) 'Missing validators'
    foreach ($set in $rule.rule.serverVariables.set) { Assert-That ($cacheHeaderNames -ccontains $set.GetAttribute('name') -and $set.GetAttribute('value') -ceq '') 'Wrong request header replacement' }
    foreach ($case in @(
        @('/app/second.html','GET','text/html,application/xhtml+xml',$true),
        @('/app/second.html','GET','application/json',$false),
        @('/app/second.html','POST','text/html',$false),
        @('/app/excluded/file.html','GET','text/html',$false),
        @('/app/__rumable__/test/init.js','GET','text/html',$false),
        @('/other/file.html','GET','text/html',$false)
    )) {
        $values=@{'{URL}'=$case[0];'{REQUEST_METHOD}'=$case[1];'{HTTP_ACCEPT}'=$case[2]};$matches=$true
        foreach ($condition in $rule.rule.conditions.add) {
            $options=if($condition.GetAttribute('ignoreCase') -ceq 'false'){[Text.RegularExpressions.RegexOptions]::None}else{[Text.RegularExpressions.RegexOptions]::IgnoreCase}
            $ok=[regex]::IsMatch($values[$condition.GetAttribute('input')],$condition.GetAttribute('pattern'),$options)
            if ($condition.GetAttribute('negate') -ceq 'true') {$ok=-not $ok};$matches=$matches -and $ok
        }
        Assert-That ($matches -eq $case[3]) ('Wrong guard scope: '+($case -join ' '))
    }
    foreach ($mode in @('absent','local','inherited','mixed')) {
        $allowedVariables=New-Object CacheCollection;$inboundRules=New-Object CacheCollection
        foreach ($name in @('parent-business','local-business')) { $e=$inboundRules.CreateElement('rule');$e.SetAttributeValue('name',$name);[void]$inboundRules.Add($e) }
        $local=''
        for ($i=0;$i -lt $cacheHeaderNames.Count;$i++) {
            $name=$cacheHeaderNames[$i]
            if ($mode -ne 'absent') { $e=$allowedVariables.CreateElement('add');$e.SetAttributeValue('name',$name);[void]$allowedVariables.Add($e) }
            if ($mode -eq 'local' -or ($mode -eq 'mixed' -and $i -eq 0)) { $local+='<add name="'+$name+'" />' }
        }
        $e=$allowedVariables.CreateElement('add');$e.SetAttributeValue('name','HTTP_BUSINESS');[void]$allowedVariables.Add($e)
        $source='<configuration><location path="Site/app"><system.webServer><rewrite><rules><rule name="local-business" /></rules><allowedServerVariables>'+$local+'</allowedServerVariables></rewrite></system.webServer></location></configuration>'
        [IO.File]::WriteAllText($configPath,$source);[xml]$xml=$source
        $before=Read-CacheGuard $xml.SelectSingleNode('/configuration/location')
        $wanted=New-EnabledCacheGuard $before
        $roundtrip=$wanted | ConvertTo-Json -Depth 30 | ConvertFrom-Json
        Assert-That (Test-CacheGuard $wanted $roundtrip) 'Guard JSON roundtrip lost state'
        $output=@(Restore-CacheGuard $roundtrip $before)
        Assert-That ($output.Count -eq 0) 'MWA return values contaminated JSON'
        Assert-That ($inboundRules.Count -eq 3 -and $inboundRules[0].GetAttributeValue('name') -ceq 'parent-business' -and $inboundRules[1].GetAttributeValue('name') -ceq ($prefix+'revalidate') -and $inboundRules[2].GetAttributeValue('name') -ceq 'local-business') 'Guard reordered business rules or ran after local rules'
        Assert-That ($inboundRules[1].GetChildElement('serverVariables').GetCollection().Count -eq 2) 'Import lost server-variable set elements'
        Restore-CacheGuard $wanted $wanted
        Assert-That ($inboundRules.Count -eq 3 -and $allowedVariables.Count -eq 3) 'Repeat apply duplicated rule/permissions'
        Restore-CacheGuard $before $wanted
        Assert-That ($inboundRules.Count -eq 2 -and $inboundRules[1].GetAttributeValue('name') -ceq 'local-business') 'Disable did not restore local business order'
        $count=if($mode -eq 'absent'){1}else{3}
        Assert-That ($allowedVariables.Count -eq $count) 'Disable removed pre-existing/inherited permission or retained a new one'
        foreach ($field in @('rules','permissions','effectivePermissions','ruleIndex')) {
            $bad=$wanted.Clone()
            if($field -eq 'rules'){$bad.rules=@($bad.rules[0].Replace('value=""','value="external"'))}
            elseif($field -eq 'ruleIndex'){$bad.ruleIndex=1}
            else{$bad[$field]=@()}
            # In inherited mode the local permission list is already empty.
            if($field -eq 'permissions' -and @($wanted.permissions).Count -eq 0){$bad.permissions=@('HTTP_IF_NONE_MATCH')}
            Assert-That (-not (Test-CacheGuard $wanted $bad)) ('Drift ignored: '+$field)
        }
    }
    Assert-That (Test-CacheGuard $null @{rules=@();permissions=@();effectivePermissions=@();ruleIndex=-1}) 'Legacy empty guard was rejected'
    Assert-That (-not (Test-CacheGuard $null $wanted)) 'Legacy recovery ignored unexpected inbound rule'
    # Missing bridge acknowledgement: recognize only the complete committed
    # guard and reject partial permission restoration or missing inbound rules.
    $original=@{rules=@();preconditions=@();virtualDirectory=$null;compressionPresent=$false;compressionValue=$null;cacheGuard=$before}
    $request.original=$original;$request.previous=$original;$request.enabled=$true
    $nodes=Get-RumNodes;$v=New-Object Xml.XmlDocument;$element=$v.CreateElement('virtualDirectory');$element.SetAttribute('path',$vpath);$element.SetAttribute('physicalPath',(Join-Path $request.root 'public'))
    $live=@{rules=$nodes.rules;preconditions=$nodes.preconditions;virtualDirectory=$element.OuterXml;compressionPresent=$true;compressionValue='false';cacheGuard=$wanted}
    Assert-PendingRumSnapshot $live
    $bad=$live.Clone();$bad.cacheGuard=$before;$rejected=$false
    try { Assert-PendingRumSnapshot $bad } catch { $rejected=$true }
    Assert-That $rejected 'Pending recovery accepted missing guard'
    $request.enabled=$false;$request.previous=$live
    $disabled=$original.Clone();$disabled.virtualDirectory=$element.OuterXml
    Assert-PendingRumSnapshot $disabled
    Write-Host 'PASS: conditional navigation scope, real set import, rule order, permission ownership/inheritance, repeat/disable, drift, legacy and lost-ack recovery (mock IIS).'
} finally { [IO.File]::Delete($configPath) }
