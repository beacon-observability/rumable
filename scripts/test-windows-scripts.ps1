#requires -Version 3.0
param([string]$Binary, [string]$FixtureBaseUrl)
$ErrorActionPreference = 'Stop'
# Windows has TEMP; the portable Linux contract host may not.
if (-not $env:TEMP) { $env:TEMP = [IO.Path]::GetTempPath() }
. (Join-Path $PSScriptRoot 'windows-iis-helpers.ps1')
foreach ($path in @('windows-smoke.ps1','windows-iis-helpers.ps1','windows-cleanup.ps1')) {
    $tokens = $null; $errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $path), [ref]$tokens, [ref]$errors)
    Assert-That ($errors.Count -eq 0) ($errors | Out-String)
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) ('rumable-script-check-' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($temporary)
try {
    $configPath = Join-Path $temporary 'acceptance.toml'
    $envName = 'portable-check-' + [char]0x4e2d + [char]0x6587
    $config = New-TestConfig 'MiXeD Site' 'test' '/App' 19182 @('/App/index.html','/App/chunked.aspx') $false $envName $true
    Write-Utf8 $configPath $config
    $bytes = [IO.File]::ReadAllBytes($configPath)
    Assert-That (-not ($bytes[0] -eq 239 -and $bytes[1] -eq 187 -and $bytes[2] -eq 191)) 'Config unexpectedly has a BOM.'
    Assert-That ((Get-SHA256 ([Text.Encoding]::UTF8.GetBytes('abc'))) -eq 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad') 'SHA-256 contract failed.'
    $savedSystemRoot = $env:SystemRoot
    try {
        $env:SystemRoot = $temporary
        foreach ($app in @('/', '/app')) {
            $target = 'Test'; if ($app -ne '/') { $target += $app }
            foreach ($node in @('', '<urlCompression />', '<urlCompression doStaticCompression="false" />')) {
                Write-Utf8 (Join-Path $temporary 'System32/inetsrv/config/applicationHost.config') ('<configuration><location path="'+$target+'"><system.webServer>'+$node+'</system.webServer></location></configuration>')
                $local = Get-TestLocalCompression 'test' $app
                Assert-That ($local.present -eq $node.Contains('doStaticCompression')) 'Acceptance helper lost empty/root/subapp compression state.'
                if ($local.present) { Assert-That ($local.value -ceq 'false') 'Acceptance helper changed local compression value.' }
            }
        }
    } finally { $env:SystemRoot = $savedSystemRoot }
    & {
        function Invoke-TestHttp([string]$Url, [string]$Encoding='identity', [switch]$UseCache) {
            $script:variantCalls++
            if ($script:variantCalls -gt 1) { Assert-That ($UseCache -and $Encoding -ceq 'gzip, deflate, br, zstd') 'Acceptance bypassed browser cache variant.' }
            $text = '<script data-rumable="test" src="/init.js"></script>'
            if ($script:missWarmCache -and $script:variantCalls -eq 4) { $text = '<html><head></head></html>' }
            return @{status=200;headers=@{'X-Rumable-Parent'='parent';'X-Rumable-Child'='child';'Content-Encoding'='gzip'};text=$text}
        }
        foreach ($script:missWarmCache in @($false, $true)) {
            $script:variantCalls=0; $rejected=$false
            try { [void](Assert-TestPage 'http://127.0.0.1' '/index.html' 'test' @{current=@{script='/init.js'}} $true 'parent') }
            catch { if ($_.Exception.Message -notlike 'Expected one current injection:*') { throw }; $rejected=$true }
            Assert-That ($script:variantCalls -eq 4 -and $rejected -eq $script:missWarmCache) 'Acceptance failed to detect warm cached HTML without injection.'
        }
    }
    if ($Binary) {
        $Binary = (Resolve-Path -LiteralPath $Binary).Path
        $log = Join-Path $temporary 'cli.log'
        Write-Utf8 $log ''
        $stateDir = Join-Path $temporary 'state'
        [void](Invoke-TestCli $Binary @('render','--config',$configPath,'--site','test','--state-dir',$stateDir) $log)
        $state = Read-TestState $stateDir 'test'
        Assert-That ($state.current.site.iis.site -ceq 'mixed site' -and $state.current.site.iis.application -ceq '/App') 'IIS case/path config was lost.'
        Assert-That ($state.current.site.probePaths.Count -eq 2 -and $state.current.site.probePaths[1] -ceq '/App/chunked.aspx') 'Probe paths changed in TOML.'
        Assert-That (-not $state.current.site.iis.allowCompressionChanges -and $state.current.site.rum.sessionSampleRate -eq 0 -and $state.current.site.rum.sessionReplayEnabled) 'Explicit false/zero/replay values changed.'
        Assert-That ($state.current.site.rum.env -ceq $envName) 'UTF-8 config value changed.'
        $key = Get-DeploymentKey $state
        $state | Add-Member -MemberType NoteProperty -Name lastCheck -Value @{ arbitrary = 'not deployment state' }
        Assert-That ((Get-DeploymentKey $state) -ceq $key) 'Check metadata changed deployment key.'
        $state.current.enabled = $true
        Assert-That ((Get-DeploymentKey $state) -cne $key) 'Deployment change was hidden.'
        [void](Invoke-TestCli $Binary @('render','--config',$configPath,'--site','test','--state-dir',$stateDir) $log -ExpectFailure)
        $missingPath = Join-Path $temporary ($envName + '.missing')
        $encodingBefore = [Console]::OutputEncoding.CodePage
        $errorResult = Invoke-TestCli $Binary @('render','--config',$missingPath,'--site','test','--state-dir',$stateDir) $log -ExpectFailure
        Assert-That ($errorResult.output.Contains($envName) -and [Console]::OutputEncoding.CodePage -eq $encodingBefore) 'Native UTF-8 error/console encoding was not preserved.'
        $rejected = $false
        try { [void](Invoke-TestCli $Binary @('version') $log -ExpectFailure) } catch { $rejected = $true }
        Assert-That $rejected 'Expected-failure helper accepted a successful command.'
    }
    if ($FixtureBaseUrl) {
        $page = Invoke-TestHttp ($FixtureBaseUrl + '/forced-gzip') 'gzip'
        Assert-That ($page.status -eq 200 -and $page.headers['Content-Encoding'] -eq 'gzip' -and $page.text.Contains('Compressed upstream')) 'Gzip decoding/header evidence failed.'
        $page = Invoke-TestHttp ($FixtureBaseUrl + '/stream')
        Assert-That ($page.status -eq 200 -and $page.headers['Transfer-Encoding'] -match 'chunked' -and $page.text.Contains('Streamed')) 'Chunked HTTP boundary failed.'
        $page = Invoke-TestHttp ($FixtureBaseUrl + '/status/503')
        Assert-That ($page.status -eq 503 -and $page.text.Contains('intentional')) 'HTTP error status/body was hidden.'
        $page = Invoke-TestHttp ($FixtureBaseUrl + '/redirect')
        Assert-That ($page.status -eq 302) 'Acceptance HTTP client followed a redirect.'
        $rejected = $false
        try { [void](Invoke-TestHttp 'https://example.com/') } catch { $rejected = $true }
        Assert-That $rejected 'Acceptance client allowed a non-loopback destination.'
    }
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or $PSVersionTable.PSEdition -eq 'Core') {
        $hostExe = 'pwsh'
        if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) { $hostExe = 'pwsh.exe' }
        $guardDir = Join-Path $temporary 'guard-report'
        [void](Invoke-TestCli (Join-Path $PSHOME $hostExe) @('-NoProfile','-File',(Join-Path $PSScriptRoot 'windows-smoke.ps1'),'-Binary','not-launched.exe','-ResultDir',$guardDir) (Join-Path $temporary 'guard.log') -ExpectFailure)
        $guard = [IO.File]::ReadAllText((Join-Path $guardDir 'report.json')) | ConvertFrom-Json
        Assert-That ($guard.result -eq 'failed' -and $guard.steps[0].name -eq 'environment-and-binary' -and $guard.cleanup.result -eq 'removed') 'Unsupported-host guard did not preserve an honest failure report.'
    }
    Write-Host ('Portable script boundaries passed; CLI=' + [bool]$Binary + ', HTTP=' + [bool]$FixtureBaseUrl + '. Windows/IIS API runtime is not verified by this check.')
} finally { Remove-Item -LiteralPath $temporary -Recurse -Force }
