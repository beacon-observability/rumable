$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
function Assert-That($value,[string]$message) { if (-not $value) { throw $message } }
$tokens=$null;$errors=$null
$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'),[ref]$tokens,[ref]$errors)
foreach ($name in @('Read-Snapshot','Restore-Compression','Assert-Snapshot')) {
    $node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name},$true)
    Invoke-Expression $node.Extent.Text
}
Add-Type @'
using System.Collections;
public class CompressionAttribute {
    public Hashtable Values; public string Name;
    public void Delete() { Values.Remove(Name); }
}
public class CompressionSection {
    public Hashtable Values = new Hashtable();
    public bool IsLocked = false;
    public int Writes = 0;
    public void SetAttributeValue(string name, object value) { Values[name]=value; Writes++; }
    public object GetAttributeValue(string name) { return Values[name]; }
    public CompressionAttribute GetAttribute(string name) { Writes++; return new CompressionAttribute { Values=Values, Name=name }; }
}
'@
$configPath=[IO.Path]::GetTempFileName()
$site=@{id='test';publicURL='http://127.0.0.1/';includePaths=@('/');excludePaths=@();iis=@{site='Site';application='/';allowCompressionChanges=$false}}
$target='Site';$prefix='rumable-test-';$vpath='/__rumable__'
try {
    foreach ($static in @('inherit','true','false')) {
        foreach ($dynamic in @('inherit','true','false')) {
            $attributes=''
            if ($static -ne 'inherit') { $attributes+=' doStaticCompression="'+$static+'"' }
            if ($dynamic -ne 'inherit') { $attributes+=' doDynamicCompression="'+$dynamic+'"' }
            [IO.File]::WriteAllText($configPath, ('<configuration><location path="Site"><system.webServer><urlCompression'+$attributes+' /></system.webServer></location></configuration>'))
            $original=Read-Snapshot | ConvertTo-Json -Depth 30 | ConvertFrom-Json
            Assert-That ($original.dynamicCompressionPresent -eq ($dynamic -ne 'inherit')) 'Dynamic inheritance was not captured'
            if ($dynamic -ne 'inherit') { Assert-That ($original.dynamicCompressionValue -ceq $dynamic) 'Dynamic local value changed' }
            $compression=New-Object CompressionSection
            $compression.Values['doStaticCompression']=$false;$compression.Values['doDynamicCompression']=$false
            $active=@{compressionPresent=$true;compressionValue='false';dynamicCompressionPresent=$true;dynamicCompressionValue='false'}
            Restore-Compression $original $active
            foreach ($entry in @(@('doStaticCompression',$static),@('doDynamicCompression',$dynamic))) {
                if ($entry[1] -eq 'inherit') { Assert-That (-not $compression.Values.ContainsKey($entry[0])) 'Restore did not remove new local override' }
                else { Assert-That ($compression.Values[$entry[0]] -eq [bool]::Parse($entry[1])) 'Restore lost original explicit value' }
            }
            $writes=$compression.Writes; Restore-Compression $original $original
            Assert-That ($compression.Writes -eq $writes) 'Repeated restore wrote unchanged attributes'
            $changed=Read-Snapshot;$changed.dynamicCompressionPresent=$true;$changed.dynamicCompressionValue=if($dynamic -eq 'true'){'false'}else{'true'}
            $rejected=$false
            try { Assert-Snapshot $original $changed } catch { $rejected=$_.Exception.Message -like '*dynamic compression changed*' }
            Assert-That $rejected 'Concurrent dynamic compression change was accepted'
        }
    }
    # Old snapshots leave the then-unowned dynamic setting untouched.
    $compression=New-Object CompressionSection;$compression.Values['doDynamicCompression']=$true
    $legacy=@{compressionPresent=$false;compressionValue=$null}
    Restore-Compression $legacy $legacy
    Assert-That ($compression.Values['doDynamicCompression'] -eq $true -and $compression.Writes -eq 0) 'Legacy recovery claimed dynamic compression'

    # Exercise the actual bridge check: dynamic-only compression needs consent,
    # while disable/recovery must not request a new enabling permission.
    $compression.Values['doStaticCompression']=$false
    $rumWanted=$true;$rewrite=@{IsLocked=$false};$application=@{VirtualDirectories=@{}}
    $request=@{checkingEnabled=$true;root=[IO.Path]::GetTempPath()}
    function Get-RumAssetReaders { }
    function Assert-CacheGuardWritable { }
    $before=$legacy
    $branch=$ast.Find({param($n) $n -is [Management.Automation.Language.IfStatementAst] -and @($n.Clauses | Where-Object { $_.Item1.Extent.Text -eq "`$request.action -eq 'check'" }).Count -gt 0},$true)
    $body=@($branch.Clauses | Where-Object { $_.Item1.Extent.Text -eq "`$request.action -eq 'check'" })[0].Item2.Extent.Text
    $run=[scriptblock]::Create($body.Substring(1,$body.Length-2))
    $rejected=$false
    try { & $run | Out-Null } catch { $rejected=$_.Exception.Message -like '*Static or dynamic compression is enabled*' }
    Assert-That $rejected 'Dynamic-only enabling bypassed consent'
    $request.checkingEnabled=$false; & $run | Out-Null
    $request.checkingEnabled=$true;$site.iis.allowCompressionChanges=$true; & $run | Out-Null

    # Exact reconstruction of a committed apply/disable whose bridge response
    # was lost. Once owned, dynamic compression must participate in drift checks.
    $original=@{rules=@();preconditions=@();virtualDirectory=$null;compressionPresent=$false;compressionValue=$null;dynamicCompressionPresent=$true;dynamicCompressionValue='true'}
    $request=@{enabled=$true;root=[IO.Path]::GetTempPath();script='/__rumable__/test/init.hash.js';original=$original;previous=$original}
    $nodes=Get-RumNodes
    $doc=New-Object Xml.XmlDocument;$v=$doc.CreateElement('virtualDirectory');$v.SetAttribute('path',$vpath);$v.SetAttribute('physicalPath',(Join-Path $request.root 'public'))
    $live=@{rules=$nodes.rules;preconditions=$nodes.preconditions;virtualDirectory=$v.OuterXml;compressionPresent=$true;compressionValue='false';dynamicCompressionPresent=$true;dynamicCompressionValue='false'}
    Assert-PendingRumSnapshot ($live | ConvertTo-Json -Depth 30 | ConvertFrom-Json)
    $bad=$live.Clone();$bad.dynamicCompressionValue='true';$rejected=$false
    try { Assert-PendingRumSnapshot $bad } catch { $rejected=$true }
    Assert-That $rejected 'Pending apply accepted dynamic drift'
    $request.previous=$live;$request.enabled=$false
    $disabled=$original.Clone();$disabled.virtualDirectory=$v.OuterXml
    Assert-PendingRumSnapshot ($disabled | ConvertTo-Json -Depth 30 | ConvertFrom-Json)
    $bad=$disabled.Clone();$bad.dynamicCompressionValue='false';$rejected=$false
    try { Assert-PendingRumSnapshot $bad } catch { $rejected=$true }
    Assert-That $rejected 'Pending disable accepted partial restoration'
    Write-Host 'PASS: static/dynamic inheritance matrix, exact restore, dynamic-only consent, legacy journals, pending apply/disable and drift rejection (mock IIS).'
} finally { [IO.File]::Delete($configPath) }
