$ErrorActionPreference = 'Stop'
# Exercise XML ownership without requiring the Windows-only IIS configuration API.
$bridge = Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'
$tokens = $null; $parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($bridge, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$read = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Read-Snapshot' }, $true)
Invoke-Expression $read.Extent.Text
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
    $target = 'Other Site'; $site.iis.site = 'Other Site'
    $empty = Read-Snapshot
    if ($empty.compressionPresent -or $empty.rules.Count -or $null -ne $empty.virtualDirectory) { throw 'Snapshot captured another site.' }
    Write-Host 'IIS snapshot: case-insensitive ownership, site isolation and JSON persistence passed.'
} finally {
    [IO.File]::Delete($configPath)
}
