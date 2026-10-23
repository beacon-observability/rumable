# Shared by the IIS acceptance runner and its portable boundary checks. PS 3 / .NET 4 only.
function Assert-That($Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Write-Utf8([string]$Path, [string]$Text) {
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($Path))
    [IO.File]::WriteAllText($Path, $Text, (New-Object Text.UTF8Encoding($false)))
}

function Get-SHA256([byte[]]$Bytes) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash($Bytes))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Invoke-TestHttp([string]$Url, [string]$Encoding = 'identity', [switch]$UseCache, [hashtable]$Validators = @{}) {
    $uri = [uri]$Url
    Assert-That ($uri.Scheme -eq 'http' -and $uri.Host -eq '127.0.0.1') 'Acceptance HTTP requests must stay on loopback.'
    $request = [Net.HttpWebRequest]::Create($uri)
    $request.Proxy = $null
    $request.AllowAutoRedirect = $false
    $request.KeepAlive = $false
    $request.Timeout = 5000
    $request.ReadWriteTimeout = 5000
    $request.Headers['Accept-Encoding'] = $Encoding
    $request.Accept = 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8'
    if ($Validators['ETag']) { $request.Headers['If-None-Match'] = $Validators['ETag'] }
    if ($Validators['Last-Modified']) { $request.IfModifiedSince = [DateTime]::Parse($Validators['Last-Modified'], [Globalization.CultureInfo]::InvariantCulture).ToUniversalTime() }
    if ($Validators.Count) { $request.Headers['Cache-Control'] = 'max-age=0' }
    if (-not $UseCache) { $request.Headers['Cache-Control'] = 'no-cache' }
    $request.AutomaticDecompression = [Net.DecompressionMethods]::None
    $response = $null
    try {
        try { $response = $request.GetResponse() }
        catch [Net.WebException] {
            if ($null -eq $_.Exception.Response) { throw }
            $response = $_.Exception.Response
        }
        $memory = New-Object IO.MemoryStream
        try {
            $response.GetResponseStream().CopyTo($memory)
            [byte[]]$bytes = $memory.ToArray()
        } finally { $memory.Dispose() }
        $headers = @{}
        foreach ($key in $response.Headers.AllKeys) { $headers[$key] = $response.Headers[$key] }
        $wireLength = $bytes.Length
        if ($response.ContentEncoding -eq 'gzip') {
            $inputStream = New-Object IO.MemoryStream(,$bytes)
            $gzip = New-Object IO.Compression.GZipStream($inputStream, [IO.Compression.CompressionMode]::Decompress)
            $outputStream = New-Object IO.MemoryStream
            try { $gzip.CopyTo($outputStream); $bytes = $outputStream.ToArray() }
            finally { $gzip.Dispose(); $inputStream.Dispose(); $outputStream.Dispose() }
        }
        return @{ status = [int]$response.StatusCode; headers = $headers; bytes = $bytes; text = [Text.Encoding]::UTF8.GetString($bytes); wireBytes = $wireLength }
    } finally { if ($null -ne $response) { $response.Close() } }
}

function Invoke-TestCli([string]$Exe, [string[]]$Arguments, [string]$Log, [switch]$ExpectFailure) {
    $savedPreference = $ErrorActionPreference
    $savedEncoding = [Console]::OutputEncoding
    try {
        # Windows PowerShell wraps native stderr as ErrorRecord even for expected failures.
        $ErrorActionPreference = 'Continue'
        [Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
        $lines = @(& $Exe @Arguments 2>&1)
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedPreference; [Console]::OutputEncoding = $savedEncoding }
    $text = ($lines | Out-String)
    [IO.File]::AppendAllText($Log, ('> ' + $Exe + ' ' + ($Arguments -join ' ') + [Environment]::NewLine + $text), (New-Object Text.UTF8Encoding($false)))
    if ($ExpectFailure) { Assert-That ($code -ne 0) ('Expected CLI rejection: ' + ($Arguments -join ' ')) }
    else { Assert-That ($code -eq 0) ('CLI exit ' + $code + ': ' + $text) }
    return @{ exitCode = $code; output = $text }
}

function New-TestConfig([string]$SiteName, [string]$Id, [string]$App, [int]$Port, [string[]]$Paths,
                        [bool]$AllowCompression = $true, [string]$EnvName = 'acceptance', [bool]$Replay = $false) {
    $quotedPaths = @($Paths | ForEach-Object { '"' + $_ + '"' }) -join ', '
    return (@(
        'schemaVersion = 1', '[[sites]]', ('id = "' + $Id + '"'),
        ('publicURL = "http://127.0.0.1:' + $Port + $App + '/"'),
        ('probePaths = [' + $quotedPaths + ']'), ('includePaths = ["' + $App + '/"]'),
        ('excludePaths = ["' + $App + '/excluded/"]'), '[sites.iis]',
        ('site = "' + $SiteName.ToLowerInvariant() + '"'), ('application = "' + $App + '"'),
        ('allowCompressionChanges = ' + $AllowCompression.ToString().ToLowerInvariant()),
        '[sites.rum]', 'applicationId = "rumable-iis-acceptance"', 'datakitOrigin = "http://127.0.0.1:1"',
        ('env = "' + $EnvName + '"'), 'sessionSampleRate = 0', 'telemetrySampleRate = 0',
        ('sessionReplayEnabled = ' + $Replay.ToString().ToLowerInvariant()), 'sessionReplaySampleRate = 37.5'
    ) -join [Environment]::NewLine) + [Environment]::NewLine
}

function Read-TestState([string]$StateDir, [string]$Id) {
    return ([IO.File]::ReadAllText((Join-Path $StateDir ('sites/' + $Id + '/state.json'))) | ConvertFrom-Json)
}

function Get-DeploymentKey($State) {
    # check records change on a rejected check; deployment/history must not.
    return (@{ current = $State.current; previous = $State.previous; originalIIS = $State.originalIIS } | ConvertTo-Json -Depth 60 -Compress)
}

function Get-TestLocalCompression([string]$SiteName, [string]$App) {
    [xml]$xml = [IO.File]::ReadAllText((Join-Path $env:SystemRoot 'System32/inetsrv/config/applicationHost.config'))
    $target = $SiteName
    if ($App -ne '/') { $target += $App }
    $location = $xml.SelectNodes('/configuration/location') | Where-Object { $_.GetAttribute('path') -ieq $target } | Select-Object -First 1
    $compression = $null
    if ($null -ne $location) { $compression = $location.SelectSingleNode('system.webServer/urlCompression') }
    $present = $null -ne $compression -and $compression.HasAttribute('doStaticCompression')
    $value = $null
    if ($present) { $value = $compression.GetAttribute('doStaticCompression') }
    $dynamicPresent = $null -ne $compression -and $compression.HasAttribute('doDynamicCompression')
    $dynamicValue = $null
    if ($dynamicPresent) { $dynamicValue = $compression.GetAttribute('doDynamicCompression') }
    return @{ present = [bool]$present; value = $value; dynamicPresent = [bool]$dynamicPresent; dynamicValue = $dynamicValue }
}

function Get-TestBusinessRules([string]$SiteName) {
    [xml]$xml = [IO.File]::ReadAllText((Join-Path $env:SystemRoot 'System32/inetsrv/config/applicationHost.config'))
    $entries = @()
    foreach ($location in $xml.configuration.location) {
        if ($location.path -ieq $SiteName -or $location.path.StartsWith($SiteName + '/', [StringComparison]::OrdinalIgnoreCase)) {
            foreach ($rule in $location.'system.webServer'.rewrite.outboundRules.rule) {
                if ($null -ne $rule -and -not $rule.name.StartsWith('rumable-', [StringComparison]::Ordinal)) {
                    $entries += $location.path + ':' + $rule.OuterXml
                }
            }
        }
    }
    return (($entries | Sort-Object) -join [Environment]::NewLine)
}

function Get-TestEffective([string]$SiteName, [string]$App) {
    $manager = New-Object Microsoft.Web.Administration.ServerManager
    try {
        $config = $manager.GetApplicationHostConfiguration()
        $compression = $config.GetSection('system.webServer/urlCompression', ($SiteName + $App))
        return @{
            static = [bool]$compression.GetAttributeValue('doStaticCompression')
            dynamic = [bool]$compression.GetAttributeValue('doDynamicCompression')
            rewriteBeforeCache = [bool]$config.GetSection('system.webServer/rewrite/outboundRules', ($SiteName + $App)).GetAttributeValue('rewriteBeforeCache')
        }
    } finally { $manager.Dispose() }
}

function Add-TestHeaderRule($Config, [string]$Target, [string]$Name, [string]$Header, [string]$Value) {
    $rules = $Config.GetSection('system.webServer/rewrite/outboundRules', $Target).GetCollection()
    $rule = $rules.CreateElement('rule')
    $rule.SetAttributeValue('name', $Name)
    $match = $rule.GetChildElement('match')
    $match.SetAttributeValue('serverVariable', $Header)
    $match.SetAttributeValue('pattern', '.*')
    $action = $rule.GetChildElement('action')
    $action.SetAttributeValue('type', 'Rewrite')
    $action.SetAttributeValue('value', $Value)
    $rules.Add($rule)
}

function Set-TestRuleValue([string]$SiteName, [string]$App, [string]$Name, [string]$Value) {
    $manager = New-Object Microsoft.Web.Administration.ServerManager
    try {
        $rules = $manager.GetApplicationHostConfiguration().GetSection('system.webServer/rewrite/outboundRules', ($SiteName + $App)).GetCollection()
        $rule = @($rules | Where-Object { $_.GetAttributeValue('name') -ceq $Name }) | Select-Object -First 1
        Assert-That ($null -ne $rule) ('Rule not found: ' + $Name)
        $rule.GetChildElement('action').SetAttributeValue('value', $Value)
        $manager.CommitChanges()
    } finally { $manager.Dispose() }
}

function Assert-TestPage([string]$BaseUrl, [string]$Path, [string]$Id, $State, [bool]$Enabled, [string]$ParentValue) {
    $identity = $null
    foreach ($attempt in @(0,1,2,3)) {
        if ($attempt -eq 3) { $page = Invoke-TestHttp ($BaseUrl + $Path); $identity = $page }
        else { $page = Invoke-TestHttp ($BaseUrl + $Path) 'gzip, deflate, br, zstd' -UseCache }
        $evidence = ' variant=' + $attempt + ' headers=' + ($page.headers | ConvertTo-Json -Compress)
        Assert-That ($page.status -eq 200) ('HTTP ' + $page.status + ': ' + $Path)
        Assert-That ($page.headers['X-Rumable-Parent'] -eq $ParentValue) 'Inherited business rule changed.'
        Assert-That ($page.headers['X-Rumable-Child'] -eq 'child') 'Local business rule changed.'
        $count = [regex]::Matches($page.text, ('data-rumable="' + $Id + '"')).Count
        if ($Enabled) {
            Assert-That ($count -eq 1 -and $page.text.Contains($State.current.script)) ('Expected one current injection: ' + $Path + $evidence)
            Assert-That (-not $page.headers['ETag'] -or $page.headers['ETag'].StartsWith('W/')) 'Rewritten HTML kept a strong ETag.'
        } else { Assert-That ($count -eq 0) ('Injection survived disable: ' + $Path + $evidence) }
    }
    return $identity
}

function Assert-TestAssets([string]$BaseUrl, [string]$Id, $State) {
    foreach ($asset in $State.current.assets.PSObject.Properties) {
        $path = $State.current.site.publicURL
        $prefix = ([uri]$path).AbsolutePath + '__rumable__/' + $Id + '/'
        $leaf = $asset.Name.Substring(('public/' + $Id + '/').Length)
        $response = Invoke-TestHttp ($BaseUrl + $prefix + $leaf)
        Assert-That ($response.status -eq 200 -and (Get-SHA256 $response.bytes) -eq $asset.Value) ('Asset missing or modified: ' + $leaf)
        if ($leaf.EndsWith('.js')) {
            $mime = ($response.headers['Content-Type'] -split ';')[0].ToLowerInvariant()
            Assert-That (@('application/javascript', 'text/javascript', 'application/x-javascript') -contains $mime) ('Invalid JS MIME: ' + $mime)
        }
    }
}

function Assert-TestStaticGzip([string]$Url, [bool]$Expected) {
    for ($i=0; $i -lt 12; $i++) {
        $page = Invoke-TestHttp $Url 'gzip'
        Assert-That ($page.status -eq 200 -and $page.text.Contains('IIS acceptance')) 'Static baseline is not readable HTML.'
        $compressed = $page.headers['Content-Encoding'] -eq 'gzip'
        if ($compressed -eq $Expected) { return }
        if (-not $Expected) { break }
        Start-Sleep -Milliseconds 250
    }
    throw 'Static gzip disagrees with the expected baseline; review httpCompression MIME/min-size/CPU/hit-frequency settings.'
}

function Remove-TestDeployment([string]$SiteName, [string]$Work) {
    Assert-That ($SiteName -match '^Rumable-Acceptance-[0-9]{8}-[0-9]{6}-[a-f0-9]{8}$') 'Not an acceptance resource name.'
    $expected = [IO.Path]::GetFullPath((Join-Path $env:TEMP $SiteName))
    Assert-That ([IO.Path]::GetFullPath($Work) -ieq $expected) 'Cleanup directory must be the original test tree under this account TEMP.'
    $manager = New-Object Microsoft.Web.Administration.ServerManager
    try {
        $site = $manager.Sites[$SiteName]
        $ownedPools=@($SiteName,($SiteName+'-inherited'),($SiteName+'-local-on'),($SiteName+'-local-off'))
        if ($null -ne $site) {
            Assert-That ($site.Applications['/'].VirtualDirectories['/'].PhysicalPath -ieq (Join-Path $Work 'www')) 'Cleanup site ownership mismatch.'
        }
        foreach ($otherSite in $manager.Sites) {
            foreach ($app in $otherSite.Applications) {
                Assert-That ($ownedPools -inotcontains $app.ApplicationPoolName -or $otherSite.Name -ieq $SiteName) 'Another site uses this pool; refusing to stop it.'
            }
        }
        if ($null -ne $site) {
            if ($site.State -ne 'Stopped') { [void]$site.Stop() }
            $manager.Sites.Remove($site)
        }
        foreach ($poolName in $ownedPools) {
            $pool = $manager.ApplicationPools[$poolName]
            if ($null -ne $pool) {
                if ($pool.State -ne 'Stopped') { [void]$pool.Stop() }
                $manager.ApplicationPools.Remove($pool)
            }
        }
        $config = $manager.GetApplicationHostConfiguration()
        foreach ($path in $config.GetLocationPaths()) {
            if ($path -ieq $SiteName -or $path.StartsWith($SiteName + '/', [StringComparison]::OrdinalIgnoreCase)) { $config.RemoveLocationPath($path) }
        }
        $manager.CommitChanges()
    } finally { $manager.Dispose() }
    if (Test-Path -LiteralPath $Work) { Remove-Item -LiteralPath $Work -Recurse -Force }
}

# Use the real validators captured before installation, including the gzip
# representation. This runs before any unconditional post-enable second-page GET.
function Assert-TestConditionalPage([string]$Url,[string]$Id,$State,$Baseline) {
    Assert-That ($Baseline.status -eq 200 -and $Baseline.headers['ETag'] -and $Baseline.headers['Last-Modified']) 'Conditional baseline must have both real validators.'
    foreach ($headers in @(@{'ETag'=$Baseline.headers['ETag']},@{'Last-Modified'=$Baseline.headers['Last-Modified']},@{'ETag'=$Baseline.headers['ETag'];'Last-Modified'=$Baseline.headers['Last-Modified']})) {
        $page=Invoke-TestHttp $Url 'gzip, deflate, br, zstd' -UseCache -Validators $headers
        Assert-That ($page.status -eq 200) ('Old browser validators reused HTTP '+$page.status)
        Assert-That ([regex]::Matches($page.text,('data-rumable="'+$Id+'"')).Count -eq 1 -and $page.text.Contains($State.current.script)) 'Conditional reload kept stale HTML or bootstrap.'
        Assert-That (-not $page.headers['Content-Encoding'] -and -not $page.headers['ETag'] -and -not $page.headers['Last-Modified']) 'Conditional HTML kept compression or validators.'
        Assert-That ($page.text.Contains('window.businessLiteral="</head>"')) 'Conditional reload corrupted business JS.'
    }
}
