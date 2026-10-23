param(
    [Parameter(Mandatory=$true)][string]$BaseUrl,
    [Parameter(Mandatory=$true)][string]$Config,
    [switch]$Discover,
    [switch]$NonInteractive
)
$ErrorActionPreference = 'Stop'

function Initialize-RumableNativeSystem {
    if ('RumableInstaller.NativeSystem' -as [type]) { return }
    # IsWow64Process2 reports the native OS architecture even under ARM64
    # emulation. GetNativeSystemInfo is only the pre-Windows-10-1709 fallback.
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
namespace RumableInstaller {
    public static class NativeSystem {
        [StructLayout(LayoutKind.Sequential)]
        private struct SystemInfo {
            public ushort Architecture, Reserved;
            public uint PageSize;
            public IntPtr MinimumAddress, MaximumAddress;
            public UIntPtr ActiveProcessorMask;
            public uint ProcessorCount, ProcessorType, AllocationGranularity;
            public ushort ProcessorLevel, ProcessorRevision;
        }
        [DllImport("kernel32.dll", SetLastError=true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool IsWow64Process2(IntPtr process, out ushort processMachine, out ushort nativeMachine);
        [DllImport("kernel32.dll")]
        private static extern void GetNativeSystemInfo(out SystemInfo info);
        public static ushort Architecture() {
            try {
                ushort processMachine, nativeMachine;
                if (!IsWow64Process2(new IntPtr(-1), out processMachine, out nativeMachine))
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                return nativeMachine;
            } catch (EntryPointNotFoundException) {
                SystemInfo info;
                GetNativeSystemInfo(out info);
                switch (info.Architecture) {
                    case 0: return 0x014c;
                    case 9: return 0x8664;
                    case 12: return 0xaa64;
                    default: throw new PlatformNotSupportedException("Unsupported native Windows architecture");
                }
            }
        }
    }
}
'@
}

function Get-RumableProgramFiles([bool]$Is64BitOS, [bool]$Is64BitProcess, [string]$ProgramFiles, [string]$ProgramW6432) {
    # A 32-bit PowerShell must not install the native CLI in Program Files (x86).
    $directory = $ProgramFiles
    if ($Is64BitOS -and -not $Is64BitProcess) { $directory = $ProgramW6432 }
    if ([string]::IsNullOrWhiteSpace($directory)) { throw 'Cannot locate the native Program Files directory' }
    return $directory
}

function Get-RumableWindowsHost {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'This installer requires Windows' }
    if ($PSVersionTable.PSVersion -lt [Version]'5.1') { throw 'Install PowerShell 5.1 or newer before running this installer' }
    $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator' }
    Initialize-RumableNativeSystem
    $machine = [RumableInstaller.NativeSystem]::Architecture()
    $architecture = switch ($machine) { 0x014c { '386' }; 0x8664 { 'amd64' }; 0xaa64 { 'arm64' }; default { throw 'Unsupported native Windows architecture' } }
    # WMI reports the actual Windows version, unaffected by application
    # compatibility manifests that can change Environment.OSVersion.Version.
    $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop
    $programFiles = Get-RumableProgramFiles ([Environment]::Is64BitOperatingSystem) ([Environment]::Is64BitProcess) $env:ProgramFiles $env:ProgramW6432
    if ([string]::IsNullOrWhiteSpace($env:ProgramData)) { throw 'Cannot locate the ProgramData directory' }
    return @{ Architecture=$architecture; Version=[Version]$os.Version; ServicePack=[int]$os.ServicePackMajorVersion; ProgramFiles=$programFiles; ProgramData=$env:ProgramData }
}

function Get-RumableWindowsPackage([string]$Architecture, [Version]$OSVersion, [int]$ServicePack, [string]$ReleaseVersion) {
    if ($Architecture -notin @('386','amd64','arm64')) { throw 'Supported Windows architectures: x86, x64 and ARM64' }
    $flavor = 'modern'
    if ($OSVersion.Major -lt 10) {
        if ($OSVersion.Major -ne 6 -or $OSVersion.Minor -notin @(1,2,3) -or ($OSVersion.Minor -eq 1 -and $ServicePack -lt 1)) {
            throw 'Requires Windows 7 SP1 / Server 2008 R2 SP1 or newer'
        }
        if ($Architecture -eq 'arm64') { throw 'Windows ARM64 requires Windows 10 or newer' }
        $flavor = 'legacy'
    }
    $hashes = @{
        '386/modern' = '@@WINDOWS_386_SHA256@@'
        'amd64/modern' = '@@WINDOWS_AMD64_SHA256@@'
        'arm64/modern' = '@@WINDOWS_ARM64_SHA256@@'
        '386/legacy' = '@@WINDOWS_386_LEGACY_SHA256@@'
        'amd64/legacy' = '@@WINDOWS_AMD64_LEGACY_SHA256@@'
    }
    $suffix = ''
    if ($flavor -eq 'legacy') { $suffix = '_legacy' }
    return @{ Architecture=$Architecture; Flavor=$flavor; Digest=$hashes[($Architecture + '/' + $flavor)]; File=('rumable_' + $ReleaseVersion + '_windows_' + $Architecture + $suffix + '.zip') }
}

function Test-RumableNonInteractive {
    if ([Console]::IsInputRedirected -or -not [Environment]::UserInteractive) { return $true }
    # PowerShell -NonInteractive does not itself redirect the child's handles.
    return @([Environment]::GetCommandLineArgs() | Where-Object { $_ -match '^-(NonI|NonInteractive)$' }).Count -gt 0
}

function Invoke-RumableSetup([string]$Binary, [string]$ConfigPath, [string]$StateDirectory, [switch]$Discover, [switch]$NonInteractive) {
    $setupArgs = @('setup','--config',$ConfigPath,'--state-dir',$StateDirectory,'--yes')
    if ($Discover) { $setupArgs += '--discover' }
    if ($NonInteractive -or (Test-RumableNonInteractive)) { $setupArgs += '--non-interactive' }
    & $Binary @setupArgs
    if ($LASTEXITCODE -ne 0) { throw 'Setup failed; retain the state directory and inspect the reported cause' }
}

function Invoke-RumableWindowsInstall([string]$BaseUrl, [string]$Config, [switch]$Discover, [switch]$NonInteractive) {
    $releaseVersion = '@@VERSION@@'
    if ($releaseVersion.Contains('@@')) { throw 'Use the CI-published installer, not the source template' }
    $source = [Uri]$BaseUrl
    if (-not $source.IsAbsoluteUri -or $source.Scheme -ne 'https' -or $source.UserInfo -or $source.Query -or $source.Fragment) { throw 'BaseUrl must be a public HTTPS directory' }
    $BaseUrl = $BaseUrl.TrimEnd('/')
    if ($BaseUrl -match '^https://github\.com/[^/]+/[^/]+/releases/latest/download$') { $BaseUrl = $BaseUrl.Substring(0, $BaseUrl.Length - '/releases/latest/download'.Length) + '/releases/download/v' + $releaseVersion }
    elseif ($BaseUrl.EndsWith('/latest', [StringComparison]::Ordinal)) { $BaseUrl = $BaseUrl.Substring(0, $BaseUrl.Length - 7) + '/' + $releaseVersion }
    $hostInfo = Get-RumableWindowsHost
    $package = Get-RumableWindowsPackage $hostInfo.Architecture $hostInfo.Version $hostInfo.ServicePack $releaseVersion
    if ($package.Architecture -eq '386') { throw 'Detected 32-bit Windows: IIS setup requires a 64-bit OS. The x86 CLI package remains available for manual configuration and probing.' }
    if ($package.Digest -notmatch '^[a-f0-9]{64}$') { throw 'Installer package checksum is missing; use the CI-published installer' }
    $bytes = [Convert]::FromBase64String($Config)
    if ($bytes.Length -eq 0) { throw 'Empty configuration' }
    Write-Host ('Detected Windows ' + $hostInfo.Version + ' / ' + $package.Architecture + '; selected ' + $package.File)
    $work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
    [void][IO.Directory]::CreateDirectory($work)
    try {
        [IO.File]::WriteAllBytes((Join-Path $work 'rumable.toml'), $bytes)
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $download = $BaseUrl + '/' + $package.File
        Invoke-WebRequest -UseBasicParsing -Uri $download -OutFile (Join-Path $work 'package.zip')
        if ((Get-FileHash (Join-Path $work 'package.zip') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $package.Digest) { throw 'Package checksum mismatch' }
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $work 'package.zip'))
        try {
            $entry = $zip.GetEntry('rumable.exe')
            if ($null -eq $entry) { throw 'Package is missing rumable.exe' }
            [IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $work 'rumable.exe'))
        } finally { $zip.Dispose() }
        $binDir = Join-Path (Join-Path $hostInfo.ProgramFiles 'Rumable') $releaseVersion
        [void][IO.Directory]::CreateDirectory($binDir)
        $bin = Join-Path $binDir 'rumable.exe'
        if (Test-Path -LiteralPath $bin) {
            if ((Get-FileHash $bin).Hash -ne (Get-FileHash (Join-Path $work 'rumable.exe')).Hash) { throw 'Installed version has different bytes' }
        } else { Copy-Item -LiteralPath (Join-Path $work 'rumable.exe') -Destination $bin }
        Invoke-RumableSetup $bin (Join-Path $work 'rumable.toml') (Join-Path $hostInfo.ProgramData 'Rumable') -Discover:$Discover -NonInteractive:$NonInteractive
    } finally { Remove-Item -LiteralPath $work -Recurse -Force }
}

Invoke-RumableWindowsInstall -BaseUrl $BaseUrl -Config $Config -Discover:$Discover -NonInteractive:$NonInteractive
