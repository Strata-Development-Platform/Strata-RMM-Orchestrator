param(
    [Parameter(Mandatory=$true)][string]$CandidateSha,
    [Parameter(Mandatory=$true)][ValidateSet('before','after')][string]$Phase,
    [string]$OutputDir = 'alpha-host-evidence',
    [string]$PackageName = ''
)
$ErrorActionPreference = 'Continue'
$stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$hostName = $env:COMPUTERNAME
$dir = Join-Path $OutputDir (Join-Path $CandidateSha (Join-Path $hostName ("$Phase-$stamp")))
New-Item -ItemType Directory -Force -Path $dir | Out-Null

function Capture([string]$Name, [scriptblock]$Body) {
    try { & $Body 2>&1 | Out-File -Encoding utf8 (Join-Path $dir "$Name.txt") }
    catch { $_ | Out-String | Out-File -Encoding utf8 (Join-Path $dir "$Name.txt") }
}

@(
    "candidate_sha=$CandidateSha",
    "phase=$Phase",
    "captured_at_utc=$stamp",
    "hostname=$hostName",
    "package=$PackageName"
) | Out-File -Encoding utf8 (Join-Path $dir 'manifest.txt')

Capture 'computer-info' { Get-ComputerInfo }
Capture 'os' { Get-CimInstance Win32_OperatingSystem | Format-List * }
Capture 'agent-service' { Get-Service -Name 'strata-rmm-agent' -ErrorAction SilentlyContinue | Format-List * }
Capture 'agent-process' { Get-Process | Where-Object { $_.ProcessName -match 'strata|rmm' } | Format-Table -AutoSize }
Capture 'system-events' { Get-WinEvent -FilterHashtable @{LogName='System'; StartTime=(Get-Date).AddMinutes(-30)} -ErrorAction SilentlyContinue | Select-Object -First 200 TimeCreated,Id,LevelDisplayName,ProviderName,Message | Format-List }

Capture 'pending-updates' {
    $session = New-Object -ComObject Microsoft.Update.Session
    $searcher = $session.CreateUpdateSearcher()
    $result = $searcher.Search('IsInstalled=0 AND IsHidden=0')
    $items = foreach ($u in $result.Updates) {
        [pscustomobject]@{ Title=$u.Title; KB=($u.KBArticleIDs -join ','); Severity=$u.MsrcSeverity; RebootBehavior=$u.InstallationBehavior.RebootBehavior }
    }
    $items | Format-Table -AutoSize
}

Capture 'pending-reboot' {
    [pscustomobject]@{
        CBS = Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending'
        WindowsUpdate = Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'
        PendingFileRenameOperations = [bool](Get-ItemProperty 'HKLM:\SYSTEM\CurrentControlSet\Control\Session Manager' -Name PendingFileRenameOperations -ErrorAction SilentlyContinue)
    } | Format-List
}

Capture 'installed-products' {
    Get-ItemProperty HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*,HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\* -ErrorAction SilentlyContinue |
      Where-Object DisplayName |
      Select-Object DisplayName,DisplayVersion,Publisher,InstallDate |
      Sort-Object DisplayName | Format-Table -AutoSize
}
if ($PackageName) {
    Capture 'selected-package' {
        Get-ItemProperty HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*,HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\* -ErrorAction SilentlyContinue |
          Where-Object { $_.DisplayName -like "*$PackageName*" } |
          Select-Object DisplayName,DisplayVersion,Publisher,InstallDate | Format-List
    }
}

Write-Output $dir
