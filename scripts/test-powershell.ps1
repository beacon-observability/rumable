$ErrorActionPreference='Stop'
foreach ($script in Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot '../internal/rumable/assets') -Filter '*.ps1') {
    $tokens=$null; $errors=$null
    [void][Management.Automation.Language.Parser]::ParseFile($script.FullName,[ref]$tokens,[ref]$errors)
    if ($errors.Count) { throw ($errors | Out-String) }
}
foreach ($name in @('test-windows-installer.ps1','test-iis-snapshot.ps1','test-iis-rum.ps1','test-otel-iis.ps1','test-windows-scripts.ps1')) {
    & pwsh -NoLogo -NoProfile -File (Join-Path $PSScriptRoot $name)
    if ($LASTEXITCODE -ne 0) { throw ('PowerShell regression failed: ' + $name) }
}
