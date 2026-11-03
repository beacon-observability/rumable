$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../internal/rumable/assets/iis-rum.ps1')
. (Join-Path $PSScriptRoot '../internal/rumable/assets/otel-iis.ps1')
$tokens = $null; $parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../internal/rumable/assets/iis.ps1'), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-Snapshot' }, $true)
Invoke-Expression $definition.Extent.Text

# Journal JSON produces PSCustomObjects; live IIS reads produce hashtables.
# In Windows PowerShell 5.1, member enumeration on an empty object's
# PSObject.Properties.Name produces a spurious name instead of an empty list.
foreach ($case in @(
    @{ expected=@{}; actual=@{}; equal=$true },
    @{ expected=@{OTEL_SERVICE_NAME='service'}; actual=@{OTEL_SERVICE_NAME='service'}; equal=$true },
    @{ expected=@{}; actual=@{OTEL_SERVICE_NAME='service'}; equal=$false },
    @{ expected=@{OTEL_SERVICE_NAME='service'}; actual=@{}; equal=$false },
    @{ expected=@{OTEL_SERVICE_NAME='service'}; actual=@{OTEL_TRACES_EXPORTER='service'}; equal=$false },
    @{ expected=@{OTEL_SERVICE_NAME='service'}; actual=@{OTEL_SERVICE_NAME='Service'}; equal=$false }
)) {
    foreach ($expected in @($case.expected, ($case.expected | ConvertTo-Json | ConvertFrom-Json))) {
        if ((Same-OTelEnvironment $expected $case.actual) -ne $case.equal) { throw 'OTel environment equality mismatch.' }
        $saved = @{otel=@{environment=$expected}}
        $live = @{otel=@{environment=$case.actual}}
        $rejected = $false
        try { Assert-Snapshot $saved $live } catch {
            if ($_.Exception.Message -notlike '*IIS OTel environment changed before commit*') { throw }
            $rejected = $true
        }
        if ($rejected -eq $case.equal) { throw 'OTel snapshot drift check mismatch.' }
    }
}

# Minimal IIS collection double: exercise restoration without Windows IIS.
Add-Type @'
public class TestOTelEntry {
    public string name;
    public string value;
    public string GetAttributeValue(string key) { return key == "name" ? name : value; }
    public void SetAttributeValue(string key, string text) {
        if (key == "name") name = text; else value = text;
    }
}
public class TestOTelCollection : System.Collections.ObjectModel.Collection<TestOTelEntry> {
    public TestOTelEntry CreateElement(string tag) { return new TestOTelEntry(); }
}
public class TestOTelPool {
    public TestOTelCollection Environment = new TestOTelCollection();
    public TestOTelCollection GetCollection(string name) { return Environment; }
}
'@
function New-TestEntry($name, $value) {
    $entry = New-Object TestOTelEntry
    $entry.name = $name; $entry.value = $value
    return $entry
}
$pool = New-Object TestOTelPool
$collection = $pool.Environment
function Get-OTelPool { return $pool }
function Read-OTelSnapshot {
    $values = @{}
    foreach ($entry in $collection) {
        if (Test-OTelKey $entry.name) { $values[$entry.name] = $entry.value }
    }
    return @{environment=$values}
}
[void]$collection.Add((New-TestEntry 'BUSINESS_SETTING' 'preserved'))
[void]$collection.Add((New-TestEntry 'OTEL_SERVICE_NAME' 'old'))
$empty = ConvertFrom-Json '{}'
Set-OTelEnvironment $empty
if ($collection.Count -ne 1 -or $collection[0].name -cne 'BUSINESS_SETTING' -or $collection[0].value -cne 'preserved') { throw 'Empty restore did not remove only owned OTel settings.' }
Set-OTelEnvironment $empty
foreach ($values in @(@{OTEL_SERVICE_NAME='new'}, (ConvertFrom-Json '{"OTEL_SERVICE_NAME":"restored"}'))) {
    Set-OTelEnvironment $values
    if ($collection.Count -ne 2 -or -not (Same-OTelEnvironment $values (Read-OTelSnapshot).environment)) { throw 'Nonempty restore lost environment settings.' }
    $original = $collection[1]
    Set-OTelEnvironment $values
    if (-not [object]::ReferenceEquals($original, $collection[1])) { throw 'Equal environment was rewritten.' }
}
try { Set-OTelEnvironment (ConvertFrom-Json '{"BUSINESS_SETTING":"foreign"}'); throw 'Invalid snapshot key was accepted.' } catch {
    if ($_.Exception.Message -notlike '*Invalid OTel environment key in snapshot*') { throw }
}
if ($collection.Count -ne 2 -or $collection[0].value -cne 'preserved' -or $collection[1].value -cne 'restored') { throw 'Rejected snapshot changed pool settings.' }
Set-OTelEnvironment @{}
if ($collection.Count -ne 1) { throw 'Dictionary empty restore did not clear OTel settings.' }
Write-Host 'OTel environment: empty/nonempty JSON and dictionary snapshots, drift, restoration and ownership passed.'
