$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
function Read-RumRuntime { return @{pool='Dedicated';applications=@('default web site/app')} }
# Exercise XML ownership without requiring the Windows-only IIS configuration API.
$bridge = Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'
$tokens = $null; $parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($bridge, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$read = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Read-Snapshot' }, $true)
Invoke-Expression $read.Extent.Text
$restoreCompression = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Restore-Compression' }, $true)
Invoke-Expression $restoreCompression.Extent.Text
$assertSnapshot = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-Snapshot' }, $true)
Invoke-Expression $assertSnapshot.Extent.Text
$compression = [pscustomobject]@{ Writes=0; Value=$null }
$compression | Add-Member ScriptMethod SetAttributeValue { param($key,$value) $this.Writes++; $this.Value=$value }
$compression | Add-Member ScriptMethod GetAttribute { throw 'Unexpected attribute deletion on unchanged locked compression.' }
$same = @{ compressionPresent=$true; compressionValue='true' }
Restore-Compression $same ($same | ConvertTo-Json | ConvertFrom-Json)
if ($compression.Writes -ne 0) { throw 'OTel-only restore attempted to modify unchanged compression.' }
Restore-Compression $same @{ compressionPresent=$true; compressionValue='false' }
if ($compression.Writes -ne 1 -or $compression.Value -ne $true) { throw 'RUM disable did not restore changed compression.' }
$configPath = [IO.Path]::GetTempFileName()
try {
    [IO.File]::WriteAllText($configPath, @'
<configuration>
  <system.applicationHost><sites><site name="Default Web Site"><application path="/App"><virtualDirectory path="/__RUMABLE__" physicalPath="C:\Rumable\public" /></application></site></sites></system.applicationHost>
  <location path="Default Web Site/App"><system.webServer>
    <rewrite><outboundRules><rule name="rumable-test-inject"><match pattern="head" /></rule><rule name="business-rule" /><preConditions><preCondition name="rumable-test-html" /></preConditions></outboundRules></rewrite>
    <urlCompression doStaticCompression="true" doDynamicCompression="false" />
  </system.webServer></location>
</configuration>
'@)
    $site = @{ iis = @{ site = 'default web site'; application = '/app' } }
    $target = 'default web site/app'; $prefix = 'rumable-test-'; $vpath = '/__rumable__'
    $snapshot = Read-Snapshot
    if (-not $snapshot.compressionPresent -or $snapshot.compressionValue -ne 'true') { throw 'Case-insensitive IIS compression snapshot was lost.' }
    if ($snapshot.rules.Count -ne 1 -or $snapshot.preconditions.Count -ne 1) { throw 'Owned rules were not isolated.' }
    if ($null -eq $snapshot.virtualDirectory) { throw 'Case-insensitive virtual directory snapshot was lost.' }
    $roundTrip = $snapshot | ConvertTo-Json -Depth 30 -Compress | ConvertFrom-Json
    if (@($roundTrip.rules).Count -ne 1 -or $roundTrip.compressionValue -ne 'true') { throw 'Snapshot JSON round trip changed owned state.' }
    $snapshot.otel = @{pool='Dedicated';runtime='v4.0';pipeline='Integrated';x86=$false;identity='ApplicationPoolIdentity';user='';applications=@('Site/');environment=@{OTEL_SERVICE_NAME='original'}}
    $saved=$snapshot | ConvertTo-Json -Depth 20 | ConvertFrom-Json
    Assert-Snapshot $saved $snapshot
    foreach ($change in @('environment','pool','applications','runtime','pipeline','x86','identity','user')) {
        $live=$snapshot.Clone();$live.otel=$snapshot.otel.Clone()
        if ($change -eq 'environment') {$live.otel.environment=@{OTEL_SERVICE_NAME='external'}}
        elseif ($change -eq 'pool') {$live.otel.pool='AnotherPool'}
        elseif ($change -eq 'applications') {$live.otel.applications=@('Site/','Other/')}
        elseif ($change -eq 'x86') {$live.otel.x86=$true}
        else {$live.otel[$change]='external-change'}
        try {Assert-Snapshot $saved $live;throw 'Concurrent change was accepted.'} catch {if ($_.Exception.Message -notlike '*changed before commit*') {throw}}
    }
    foreach ($name in @('Remove-Owned','Restore-Nodes')) {
        $definition = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
        Invoke-Expression $definition.Extent.Text
    }
    Add-Type @'
public class TestVirtual {
    public string PhysicalPath;
}
public class TestVirtualCollection {
    public TestVirtual Value;
    public TestVirtual this[string key] { get { return Value; } }
    public void Remove(TestVirtual value) {
        if (!object.ReferenceEquals(Value,value)) throw new System.Exception("Wrong virtual directory removed");
        Value = null;
    }
}
'@
    $request = @{ root=[IO.Path]::GetTempPath(); removeMapping=$false }
    $virtuals = New-Object TestVirtualCollection
    $virtuals.Value = New-Object TestVirtual
    $virtuals.Value.PhysicalPath = Join-Path $request.root 'public'
    $application = @{ VirtualDirectories=$virtuals }
    $rules=@(); $preconditions=@(); $site.otel=$null
    $restore = @{ rules=@(); preconditions=@(); virtualDirectory=$null; compressionPresent=$true; compressionValue='true' }
    Restore-Nodes $restore
    if ($null -eq $virtuals.Value) { throw 'Ordinary rollback removed the retained asset mapping.' }
    $request.removeMapping=$true
    Restore-Nodes $restore
    if ($null -ne $virtuals.Value) { throw 'Full uninstall retained the owned asset mapping.' }
    $virtuals.Value = New-Object TestVirtual
    $virtuals.Value.PhysicalPath = Join-Path $request.root 'foreign-public'
    try { Restore-Nodes $restore; throw 'Foreign mapping removed.' } catch { if ($_.Exception.Message -notlike '*owned by another deployment*') { throw } }
    if ($null -eq $virtuals.Value) { throw 'Uninstall deleted a foreign resource mapping.' }
    # Windows 5.1 XML adaptation turns empty elements into System.String.
    # Both forms occur after Delete() removes the last local attribute.
    $fixture = [IO.File]::ReadAllText($configPath)
    foreach ($emptyCompression in @('<urlCompression />', '<urlCompression></urlCompression>', '', '<urlCompression doDynamicCompression="false" />')) {
        [IO.File]::WriteAllText($configPath, $fixture.Replace('<urlCompression doStaticCompression="true" doDynamicCompression="false" />', $emptyCompression))
        $empty = Read-Snapshot
        if ($empty.compressionPresent -or $null -ne $empty.compressionValue) { throw 'Empty/absent static compression override changed snapshot semantics.' }
        if ($empty.rules.Count -ne 1 -or $empty.preconditions.Count -ne 1) { throw 'Empty compression node lost owned rules.' }
        Assert-Snapshot ($empty | ConvertTo-Json -Depth 30 | ConvertFrom-Json) $empty
    }
    [IO.File]::WriteAllText($configPath, '<configuration><location path="Default Web Site/App"><system.webServer><rewrite><outboundRules><preConditions></preConditions></outboundRules></rewrite><urlCompression /></system.webServer></location></configuration>')
    $empty = Read-Snapshot
    if ($empty.compressionPresent -or $empty.rules.Count -or $empty.preconditions.Count -or $null -ne $empty.virtualDirectory) { throw 'Committed disable with empty XML nodes was not readable.' }
    [IO.File]::WriteAllText($configPath, $fixture)
    $target = 'Other Site'; $site.iis.site = 'Other Site'
    $empty = Read-Snapshot
    if ($empty.compressionPresent -or $empty.rules.Count -or $null -ne $empty.virtualDirectory) { throw 'Snapshot captured another site.' }
    Write-Host 'IIS snapshot: ownership, site isolation, persistence, retained mappings and full uninstall passed.'
} finally {
    [IO.File]::Delete($configPath)
}
