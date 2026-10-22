# Public SDK files remain readable after disable/rollback for cached HTML.
# Private state has a separate protected Administrators/SYSTEM DACL in Go.
function Get-RumAssetReaders {
    $pool = $manager.ApplicationPools[$application.ApplicationPoolName]
    if ($null -eq $pool) { throw 'IIS application pool not found for asset permissions.' }
    $poolUser = switch ([string]$pool.ProcessModel.IdentityType) {
        'ApplicationPoolIdentity' { 'IIS APPPOOL\' + $pool.Name }
        'LocalSystem' { 'S-1-5-18' }
        'LocalService' { 'S-1-5-19' }
        'NetworkService' { 'S-1-5-20' }
        'SpecificUser' { [string]$pool.ProcessModel.UserName }
        default { throw 'Unsupported application pool identity; review public asset access.' }
    }
    $web = $manager.GetWebConfiguration($site.iis.site, $site.iis.application)
    $anonymous = $web.GetSection('system.webServer/security/authentication/anonymousAuthentication')
    if (-not [bool]$anonymous.GetAttributeValue('enabled')) { throw 'RUM asset validation requires anonymous access; configure a public asset endpoint first.' }
    $anonymousUser = [string]$anonymous.GetAttributeValue('userName')
    if ([string]::IsNullOrEmpty($anonymousUser)) { $anonymousUser=$poolUser }
    foreach ($user in @($poolUser,$anonymousUser) | Select-Object -Unique) {
        if ($user -match '^S-1-') { New-Object Security.Principal.SecurityIdentifier($user) }
        else { (New-Object Security.Principal.NTAccount($user)).Translate([Security.Principal.SecurityIdentifier]) }
    }
}

function Set-RumPublicACL {
    $public = Join-Path $request.root 'public'
    $readers = @(Get-RumAssetReaders)
    foreach ($item in @(Get-Item -LiteralPath $public) + @(Get-ChildItem -LiteralPath $public -Recurse -Force)) {
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Public assets contain a reparse point; refusing ACL changes.' }
    }
    $acl = New-Object Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true,$false)
    foreach ($id in @('S-1-5-18','S-1-5-32-544')) {
        $sid = New-Object Security.Principal.SecurityIdentifier($id)
        $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($sid,'FullControl','ContainerInherit, ObjectInherit','None','Allow')))
    }
    foreach ($sid in $readers) {
        $acl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($sid,'ReadAndExecute','ContainerInherit, ObjectInherit','None','Allow')))
        # Traverse parents only, without granting directory listing or read of
        # state/transactions. Preserve unrelated ACEs on shared parent folders.
        foreach ($parent in @($request.root, (Split-Path $request.root), (Split-Path (Split-Path $request.root)))) {
            $parentACL = Get-Acl -LiteralPath $parent
            $parentACL.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule($sid,'Traverse','None','None','Allow')))
            Set-Acl -LiteralPath $parent -AclObject $parentACL
        }
    }
    Set-Acl -LiteralPath $public -AclObject $acl
    # Remove stale explicit permissions only inside the owned public subtree.
    foreach ($item in Get-ChildItem -LiteralPath $public -Recurse -Force) {
        $child = Get-Acl -LiteralPath $item.FullName
        $child.SetAccessRuleProtection($false,$false)
        foreach ($rule in @($child.Access | Where-Object { -not $_.IsInherited })) { [void]$child.RemoveAccessRuleSpecific($rule) }
        Set-Acl -LiteralPath $item.FullName -AclObject $child
    }
}
