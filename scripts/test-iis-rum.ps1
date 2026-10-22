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
foreach ($legacy in @($false,$true)) {
    $nodes=Get-RumNodes -LegacyNullExclude:$legacy
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
Assert-That ((ConvertTo-CanonicalOwnedXml '<add input="{URL}" pattern="/" negate="false" />') -ceq (ConvertTo-CanonicalOwnedXml '<add pattern="/" input="{URL}" />')) 'MWA default attributes do not compare'
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
    public void SetAttributeValue(string k, object v) { Values[k]=v; }
    public object GetAttributeValue(string k) { return Values[k]; }
    public ReturningCollection GetCollection() { return Collection; }
    public ReturningElement GetChildElement(string name) { return new ReturningElement(); }
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
Write-Host 'PASS: null/empty/custom path rules, subapplication paths, exact old-journal recovery, drift rejection, and MWA collection output suppression.'
