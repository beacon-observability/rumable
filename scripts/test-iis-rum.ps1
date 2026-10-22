$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
function Assert-That([bool]$condition,[string]$message) { if (-not $condition) { throw $message } }
$prefix='rumable-test-'; $vpath='/__rumable__'
$site=@{id='test';publicURL='https://example.com/';includePaths=@('/');excludePaths=$null}
$request=@{root=[IO.Path]::GetTempPath();script='/__rumable__/test/init.hash.js';enabled=$true}
$original=@{rules=@();preconditions=@();virtualDirectory=$null;compressionPresent=$false;compressionValue=$null}
$request.previous=$original; $request.original=$original
foreach ($exclusions in @($null, @(), @('/api/','/a.b/'))) {
    $site.excludePaths=$exclusions
    $nodes=Get-RumNodes
    Assert-That ($nodes.rules.Count -eq 3 -and $nodes.preconditions.Count -eq 1) 'Unexpected output from rule builder'
    foreach ($text in $nodes.rules) {
        [xml]$rule=$text
        $pattern=$rule.rule.conditions.add[1].pattern
        Assert-That (-not [regex]::IsMatch('/',$pattern)) ('Root was excluded: '+$pattern)
        Assert-That ([regex]::IsMatch('/__rumable__/test/sdk.js',$pattern)) 'SDK path was not excluded'
        if ($null -ne $exclusions -and $exclusions.Count) {
            Assert-That ([regex]::IsMatch('/api/products',$pattern)) 'Configured prefix was not excluded'
            Assert-That (-not [regex]::IsMatch('/axb/page',$pattern)) 'Regex metacharacter was not escaped'
        }
    }
}
$site.publicURL='https://example.com/app/'; $site.includePaths=@('/app/'); $site.excludePaths=$null
$nodes=Get-RumNodes
Assert-That ([regex]::IsMatch('/app/__rumable__/test/x',([xml]$nodes.rules[0]).rule.conditions.add[1].pattern)) 'Application asset exclusion is wrong'
$site.publicURL='https://example.com/'; $site.includePaths=@('/')
foreach ($injection in @('', '<script data-rumable-sdk="test" src="https://cdn.example.com/sdk.js" integrity="sha256-test" crossorigin="anonymous"></script><script data-rumable="test" src="/__rumable__/test/init.hash.js"></script>')) {
$request.injection=$injection
foreach ($legacy in @($false,$true)) {
    $nodes=Get-RumNodes -LegacyNullExclude:$legacy
    if ($injection) { Assert-That (([xml]$nodes.rules[0]).rule.action.value -ceq ('<head>'+$injection)) 'Synchronous tags changed order or attributes in IIS rule' }
    if ($legacy) { Assert-That (([xml]$nodes.rules[0]).rule.conditions.add[1].pattern -ceq '^(?:|/__rumable__/)') 'Legacy fault fixture changed' }
    $doc=New-Object Xml.XmlDocument; $v=$doc.CreateElement('virtualDirectory');$v.SetAttribute('path',$vpath);$v.SetAttribute('physicalPath',(Join-Path $request.root 'public'))
    $live=@{rules=$nodes.rules;preconditions=$nodes.preconditions;virtualDirectory=$v.OuterXml;compressionPresent=$true;compressionValue='false'}
    Assert-PendingRumSnapshot $live
    foreach ($field in @('rules','preconditions','virtualDirectory','compressionValue','otel')) {
        $bad=$live.Clone()
        if ($field -eq 'compressionValue') { $bad[$field]='true' }
        elseif ($field -eq 'otel') { $bad[$field]=@{pool='foreign'} }
        else { $bad[$field]=@('<unexpected />') }
        $rejected=$false
        try { Assert-PendingRumSnapshot $bad } catch { $rejected=$true }
        Assert-That $rejected ('Recovery accepted foreign '+$field)
    }
    $restored=$live.Clone();$restored.rules=@();$restored.preconditions=@();$restored.compressionPresent=$false;$restored.compressionValue=$null
    Assert-PendingRumSnapshot $restored
}
}
$request.injection=''
Assert-That ((ConvertTo-CanonicalOwnedXml '<add input="{URL}" pattern="/" negate="false" />') -ceq (ConvertTo-CanonicalOwnedXml '<add pattern="/" input="{URL}" />')) 'MWA default attributes do not compare'
foreach ($tag in @('rule','preCondition')) {
    $defaults = if ($tag -eq 'rule') { ' enabled="true" stopProcessing="false" patternSyntax="ECMAScript"' } else { ' logicalGrouping="MatchAll" patternSyntax="ECMAScript"' }
    Assert-That ((ConvertTo-CanonicalOwnedXml ('<'+$tag+' name="rumable-test-owned"'+$defaults+' />')) -ceq (ConvertTo-CanonicalOwnedXml ('<'+$tag+' name="rumable-test-owned" />'))) ('XML name attribute hid the '+$tag+' tag during canonical comparison')
}
Assert-That ((ConvertTo-CanonicalOwnedXml '<rule name="a" />') -cne (ConvertTo-CanonicalOwnedXml '<preCondition name="a" />')) 'Different XML tag names compared equal'
Assert-That ((ConvertTo-CanonicalOwnedXml '<rule name="a"><match pattern="x" /></rule>') -cne (ConvertTo-CanonicalOwnedXml '<rule name="a" enabled="false"><match pattern="x" /></rule>')) 'Unknown behavioral change ignored'

# Exercise the real Import-Element/Restore-Nodes against returning collection
# methods, like MWA Add (which returns ConfigurationElement, not void).
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'),[ref]$tokens,[ref]$errors)
Assert-That ($errors.Count -eq 0) ($errors | Out-String)
foreach ($name in @('Import-Element','Remove-Owned','Restore-Nodes','Restore-Compression')) {
    $definition=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name},$true)
    Invoke-Expression $definition.Extent.Text
}
Add-Type @'
using System.Collections;
public class ReturningElement {
    public Hashtable Values = new Hashtable();
    public ReturningCollection Collection = new ReturningCollection();
    public Hashtable Children = new Hashtable();
    public void SetAttributeValue(string k, object v) { Values[k]=v; }
    public object GetAttributeValue(string k) { return Values[k]; }
    public ReturningCollection GetCollection() { return Collection; }
    public ReturningElement GetChildElement(string name) {
        if (!Children.ContainsKey(name)) Children[name] = new ReturningElement();
        return (ReturningElement)Children[name];
    }
}
public class ReturningCollection : IEnumerable {
    public ArrayList Items = new ArrayList();
    public int Count { get { return Items.Count; } }
    public ReturningElement this[int i] { get { return (ReturningElement)Items[i]; } }
    public void RemoveAt(int i) { Items.RemoveAt(i); }
    public IEnumerator GetEnumerator() { return Items.GetEnumerator(); }
    public ReturningElement CreateElement(string name) { return new ReturningElement(); }
    public ReturningElement Add(ReturningElement element) { Items.Add(element); return element; }
}
public class ReturningVirtuals {
    public ReturningElement Value;
    public ReturningElement this[string key] { get { return Value; } }
    public ReturningElement Add(string path, string physicalPath) { Value=new ReturningElement(); return Value; }
}
'@
$element=New-Object ReturningElement
$output=@(Import-Element $element ([xml](Get-RumNodes).preconditions[0]).DocumentElement)
Assert-That ($output.Count -eq 0 -and $element.Collection.Count -eq 4) 'MWA Add leaked objects into bridge output'
function Set-RumPublicACL { $script:aclCalled=$true }
function Apply-OTel { }
function Read-Snapshot { return $script:fakeSnapshot }
$rules=New-Object ReturningCollection; $preconditions=New-Object ReturningCollection
$compression=New-Object ReturningElement
$application=@{VirtualDirectories=(New-Object ReturningVirtuals)}
$manager=[pscustomobject]@{Committed=$false};$manager | Add-Member ScriptMethod CommitChanges { $this.Committed=$true }
$script:fakeSnapshot=$original
$apply=$ast.Find({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and @($n.Clauses | Where-Object { $_.Item1.Extent.Text -eq "`$request.action -eq 'apply'" }).Count -gt 0 },$true)
$body=@($apply.Clauses | Where-Object { $_.Item1.Extent.Text -eq "`$request.action -eq 'apply'" })[0].Item2.Extent.Text
$request.enabled=$true
$output=@(& ([scriptblock]::Create($body.Substring(1,$body.Length-2))))
Assert-That ($manager.Committed -and $script:aclCalled -and $rules.Count -eq 3 -and $preconditions.Count -eq 1) 'Real apply branch did not build all rules'
Assert-That ($output.Count -eq 1 -and $null -ne ($output[0] | ConvertFrom-Json).rules) 'Real apply branch emitted non-JSON output'
$request.snapshot=$original
$restore=@($apply.Clauses | Where-Object { $_.Item1.Extent.Text -eq "`$request.action -eq 'restore'" })[0].Item2.Extent.Text
$output=@(& ([scriptblock]::Create($restore.Substring(1,$restore.Length-2))))
Assert-That ($output.Count -eq 1 -and $rules.Count -eq 0 -and $preconditions.Count -eq 0) 'Real restore branch leaked output or retained rules'
# Restore a real nonempty snapshot after JSON persistence (the 0.1.5 rollback
# failed here because XML's name attribute shadowed DocumentElement.Name).
$nodes=Get-RumNodes
$request.snapshot=@{rules=$nodes.rules;preconditions=$nodes.preconditions;virtualDirectory=$null;compressionPresent=$false;compressionValue=$null} | ConvertTo-Json -Depth 30 | ConvertFrom-Json
$business=$rules.CreateElement('rule');$business.SetAttributeValue('name','business-rule');[void]$rules.Add($business)
foreach ($attempt in @(1,2)) {
    $output=@(& ([scriptblock]::Create($restore.Substring(1,$restore.Length-2))))
    Assert-That ($output.Count -eq 1 -and $null -ne ($output[0] | ConvertFrom-Json).rules) 'Nonempty restore contaminated JSON output'
    Assert-That ($rules.Count -eq 4 -and $preconditions.Count -eq 1 -and $rules[0].GetAttributeValue('name') -ceq 'business-rule') 'Nonempty restore lost owned or business rules'
    Assert-That ($rules[1].GetChildElement('match').Values['occurrences'] -is [int]) 'Restore lost numeric schema type'
    Assert-That ($rules[1].GetChildElement('conditions').Collection[1].Values['negate'] -eq $true) 'Restore lost exclusion condition'
    Assert-That ($rules[2].GetChildElement('action').Values['value'] -ceq '') 'Restore lost empty response-header action'
}
foreach ($field in @('rules','preconditions')) {
    foreach ($foreign in @('<rule name="foreign" />','<preCondition name="foreign" />','<unexpected name="rumable-test-owned" />')) {
        $bad=@{rules=@();preconditions=@();virtualDirectory=$null;compressionPresent=$false;compressionValue=$null};$bad[$field]=@($foreign)
        $rejected=$false
        try { Restore-Nodes $bad } catch { $rejected=$_.Exception.Message -like 'Invalid owned * in snapshot.' }
        Assert-That $rejected ('Restore accepted foreign '+$field)
    }
}
# A disable can commit before its bridge output fails. Recovery must recognize
# exactly that result, retaining assets, and reject unrelated changes.
$request.previous=@{rules=$nodes.rules;preconditions=$nodes.preconditions;virtualDirectory=$live.virtualDirectory;compressionPresent=$true;compressionValue='false'}
$request.enabled=$false
$disabled=@{rules=@();preconditions=@();virtualDirectory=$live.virtualDirectory;compressionPresent=$false;compressionValue=$null}
Assert-PendingRumSnapshot ($disabled | ConvertTo-Json -Depth 30 | ConvertFrom-Json)
Write-Host 'PASS: null/empty/custom path rules, subapplication paths, exact old-journal recovery, drift rejection, and MWA collection output suppression.'
