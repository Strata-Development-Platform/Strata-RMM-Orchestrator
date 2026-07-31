$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$ServerUrl = if ($env:RMM_SERVER_URL) { $env:RMM_SERVER_URL.TrimEnd('/') } else { 'https://rmm.stratadevplatform.com' }
$InstallDir = if ($env:RMM_INSTALL_DIR) { $env:RMM_INSTALL_DIR } else { Join-Path $env:ProgramFiles 'Strata RMM' }
$DataDir = if ($env:RMM_DATA_DIR) { $env:RMM_DATA_DIR } else { Join-Path $env:ProgramData 'Strata RMM' }
$ConfigPath = Join-Path $DataDir 'agent.yaml'
$BinaryPath = Join-Path $InstallDir 'strata-agent.exe'
$ServiceName = 'StrataRMMAgent'

function Fail([string]$Message) { throw "Strata RMM install failed: $Message" }

$serverUri = $null
if (-not [Uri]::TryCreate($ServerUrl, [UriKind]::Absolute, [ref]$serverUri)) { Fail 'RMM_SERVER_URL must be an absolute URL' }
$isLocal = $serverUri.Host -in @('localhost', '127.0.0.1', '::1')
if ($serverUri.Scheme -ne 'https' -and -not $isLocal) { Fail 'RMM_SERVER_URL must use HTTPS outside local development' }

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Fail 'run this installer from an elevated PowerShell session'
}

$arch = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { Fail 'unsupported Windows architecture' }
}

$enrollmentToken = $env:RMM_ENROLLMENT_TOKEN
if (-not $enrollmentToken) {
    $secureToken = Read-Host 'Enrollment token' -AsSecureString
    $tokenPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)
    try { $enrollmentToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($tokenPointer) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($tokenPointer) }
}
if (-not $enrollmentToken) { Fail 'enrollment token is required' }
if ($enrollmentToken -match "[`r`n]") { Fail 'enrollment token contains invalid characters' }

$tempDir = Join-Path ([IO.Path]::GetTempPath()) ("strata-rmm-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir | Out-Null
try {
    $downloadedBinary = Join-Path $tempDir 'strata-agent.exe'
    $downloadedChecksum = Join-Path $tempDir 'strata-agent.exe.sha256'
    $binaryUrl = "$ServerUrl/releases/latest/agent/windows/$arch"

    Write-Host '==> Downloading verified Strata RMM agent'
    Invoke-WebRequest -UseBasicParsing -Uri $binaryUrl -OutFile $downloadedBinary
    Invoke-WebRequest -UseBasicParsing -Uri "$binaryUrl?checksum=sha256" -OutFile $downloadedChecksum
    $expectedHash = ((Get-Content -Raw $downloadedChecksum).Trim() -split '\s+')[0]
    if ($expectedHash -notmatch '^[a-fA-F0-9]{64}$') { Fail 'invalid checksum response' }
    $actualHash = (Get-FileHash -Algorithm SHA256 $downloadedBinary).Hash
    if ($actualHash -ne $expectedHash) { Fail 'agent checksum verification failed' }

    New-Item -ItemType Directory -Force -Path $InstallDir, $DataDir | Out-Null
    Copy-Item -Force $downloadedBinary $BinaryPath

    $escapedToken = $enrollmentToken.Replace("'", "''")
    $escapedServer = $ServerUrl.Replace("'", "''")
    $escapedData = $DataDir.Replace("'", "''")
    @"
agent:
  enrollment_token: '$escapedToken'
  register_url: '$escapedServer/api/v1/agent/register'
  log_level: info
  data_dir: '$escapedData'
collect:
  interval: 60s
  enable_system: true
store:
  type: bbolt
  path: '$escapedData\agent.db'
update:
  enabled: false
"@ | Set-Content -Encoding UTF8 -Path $ConfigPath

    & icacls.exe $DataDir /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'Administrators:(OI)(CI)F' | Out-Null
    $serviceCommand = ('"{0}" agent --config "{1}"' -f $BinaryPath, $ConfigPath)
    if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        & sc.exe config $ServiceName binPath= $serviceCommand start= auto | Out-Null
    } else {
        & sc.exe create $ServiceName binPath= $serviceCommand start= auto DisplayName= 'Strata RMM Agent' | Out-Null
        & sc.exe description $ServiceName 'Strata RMM endpoint monitoring agent' | Out-Null
    }
    Start-Service -Name $ServiceName

    $deadline = (Get-Date).AddSeconds(45)
    do {
        Start-Sleep -Seconds 2
        $service = Get-Service -Name $ServiceName
        if ($service.Status -eq 'Running') { break }
    } while ((Get-Date) -lt $deadline)
    if ($service.Status -ne 'Running') { Fail 'agent service did not remain running' }

    $runtimeConfig = Get-Content -Raw $ConfigPath
    if ($runtimeConfig -match '(?m)^\s*enrollment_token:') { Fail 'agent did not consume the one-time enrollment token' }
    Write-Host 'Strata RMM agent installed and enrolled successfully.'
} finally {
    $enrollmentToken = $null
    $env:RMM_ENROLLMENT_TOKEN = $null
    if (Test-Path $tempDir) { Remove-Item -Recurse -Force $tempDir }
}
