$ErrorActionPreference = "Stop"
. "$PSScriptRoot\isolated-services.ps1"

$root = Split-Path -Parent $PSScriptRoot
$runID = New-LLM2APITestRunID -Purpose "winservice"
$buildDirectory = Join-Path $root ".build\$runID"
$databaseName = "llm2api_winservice_$($runID.Replace('-', '_'))"
$databasePassword = "winservice-$runID-database-password"
$valkeyPassword = "winservice-$runID-valkey-password"
$environmentSnapshot = Save-LLM2APIEnvironment
$postgres = $null
$valkey = $null
$serviceInstalled = $false
$failure = $null

function Write-ServiceSecret {
  param([string] $Name, [string] $Value)
  $path = Join-Path $buildDirectory $Name
  [IO.File]::WriteAllText($path, $Value, [Text.UTF8Encoding]::new($false))
  return $path
}

Push-Location $root
try {
  New-Item -ItemType Directory -Force -Path $buildDirectory | Out-Null
  Clear-LLM2APIEnvironment
  $postgres = Start-LLM2APITestPostgres -RunID $runID -DatabaseName $databaseName -Password $databasePassword
  $valkey = Start-LLM2APITestValkey -RunID $runID -Password $valkeyPassword
  $gatewayPort = Get-LLM2APIFreeLoopbackPort
  $binaryPath = Join-Path $buildDirectory "llm2api.exe"
  $environmentPath = Join-Path $buildDirectory "service.env"

  pnpm.cmd --dir web run build
  if ($LASTEXITCODE -ne 0) { throw "Production frontend build failed." }
  go build -tags webembed -trimpath -o $binaryPath .\cmd\gateway
  if ($LASTEXITCODE -ne 0) { throw "Windows service Gateway build failed." }

  $masterKey = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("0123456789abcdef0123456789abcdef"))
  $databaseURLFile = Write-ServiceSecret "database-url" $postgres.DatabaseURL
  $valkeyPasswordFile = Write-ServiceSecret "valkey-password" $valkeyPassword
  $masterKeysFile = Write-ServiceSecret "master-keys" "1:$masterKey"
  $sessionPepperFile = Write-ServiceSecret "session-pepper" "winservice-session-pepper-$runID"
  $apiKeyPepperFile = Write-ServiceSecret "api-key-pepper" "winservice-api-key-pepper-$runID"
  $coordinationSecretFile = Write-ServiceSecret "coordination-secret" "winservice-coordination-secret-$runID"
  $lines = @(
    "LLM2API_PROFILE=production",
    "LLM2API_HTTP_ADDRESS=127.0.0.1:$gatewayPort",
    "LLM2API_DATABASE_URL_FILE=$databaseURLFile",
    "LLM2API_DATABASE_MIGRATE_ON_START=false",
    "LLM2API_DATABASE_MIN_CONNECTIONS=1",
    "LLM2API_DATABASE_MAX_CONNECTIONS=4",
    "LLM2API_VALKEY_ADDRESS=$($valkey.Address)",
    "LLM2API_VALKEY_PASSWORD_FILE=$valkeyPasswordFile",
    "LLM2API_MASTER_KEYS_FILE=$masterKeysFile",
    "LLM2API_ACTIVE_MASTER_KEY_VERSION=1",
    "LLM2API_SESSION_PEPPER_FILE=$sessionPepperFile",
    "LLM2API_API_KEY_PEPPER_FILE=$apiKeyPepperFile",
    "LLM2API_COORDINATION_KEY_HASH_SECRET_FILE=$coordinationSecretFile",
    "LLM2API_COOKIE_SECURE=true"
  )
  [IO.File]::WriteAllLines($environmentPath, $lines, [Text.UTF8Encoding]::new($false))

  foreach ($line in $lines) {
    $parts = $line -split "=", 2
    [Environment]::SetEnvironmentVariable($parts[0], $parts[1], "Process")
  }
  go run .\cmd\dbtool -action up
  if ($LASTEXITCODE -ne 0) { throw "Windows service migration preflight failed." }
  Clear-LLM2APIEnvironment

  & powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows-service.ps1 `
    -BinaryPath $binaryPath -EnvironmentFile $environmentPath -Start `
    -HealthURL "http://127.0.0.1:$gatewayPort/health/ready"
  if ($LASTEXITCODE -ne 0) { throw "Windows service installation failed." }
  $serviceInstalled = $true

  $service = Get-Service -Name LLM2API
  if ($service.Status -ne [ServiceProcess.ServiceControllerStatus]::Running -or $service.StartType -ne "Automatic") {
    throw "Windows service did not enter automatic running state."
  }
  $serviceFacts = Get-CimInstance Win32_Service -Filter "Name='LLM2API'"
  if ($serviceFacts.StartMode -ne "Auto" -or -not $serviceFacts.StartName.EndsWith("\LLM2API")) {
    throw "Windows SCM account or automatic start facts are invalid."
  }
  $event = Get-WinEvent -FilterHashtable @{ LogName = "Application"; ProviderName = "LLM2API" } -MaxEvents 20 |
    Where-Object { $_.Message -like '*gateway listening*' } | Select-Object -First 1
  if (-not $event) { throw "Gateway structured startup log did not reach Windows Event Log." }

  Stop-Service -Name LLM2API
  $service.WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(45))
  Start-Service -Name LLM2API
  $service.WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(45))
  $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$gatewayPort/health/ready" -TimeoutSec 10
  if ([int]$response.StatusCode -ne 200) { throw "Restarted Windows service was not ready." }

  & go run ./cmd/windowsservicecheck -name LLM2API
  if ($LASTEXITCODE -ne 0) { throw "Windows service restart recovery policy is invalid." }
  $evidenceDirectory = Join-Path $root ".build\acceptance-evidence"
  New-Item -ItemType Directory -Force -Path $evidenceDirectory | Out-Null
  $serviceReport = [ordered]@{
    serviceAccount = "virtual-service-account"
    fileSecrets = $true
    migrationPreflight = $true
    delayedAutomaticStart = $true
    eventLog = $true
    readiness = $true
    gracefulRestart = $true
    boundedFailureRestart = $true
  }
  [IO.File]::WriteAllText(
    (Join-Path $evidenceDirectory "windows-service-report.json"),
    ($serviceReport | ConvertTo-Json),
    [Text.UTF8Encoding]::new($false)
  )
} catch {
  $failure = $_
} finally {
  $cleanupFailures = @()
  if ($serviceInstalled -or (Get-Service -Name LLM2API -ErrorAction SilentlyContinue)) {
    try {
      & powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall-windows-service.ps1 -RemoveEventSource
      if ($LASTEXITCODE -ne 0) { throw "Windows service uninstall returned a failure." }
    } catch { $cleanupFailures += $_.Exception.Message }
  }
  try { Restore-LLM2APIEnvironment -Snapshot $environmentSnapshot } catch { $cleanupFailures += $_.Exception.Message }
  if ($null -ne $valkey) {
    try { Stop-LLM2APITestContainer -Container $valkey.Container -RunID $runID } catch { $cleanupFailures += $_.Exception.Message }
  }
  if ($null -ne $postgres) {
    try { Stop-LLM2APITestContainer -Container $postgres.Container -RunID $runID } catch { $cleanupFailures += $_.Exception.Message }
  }
  try {
    $resolvedBuild = [IO.Path]::GetFullPath($buildDirectory)
    $resolvedRoot = [IO.Path]::GetFullPath((Join-Path $root ".build"))
    if (-not $resolvedBuild.StartsWith($resolvedRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
      throw "Refusing to remove an unowned Windows service build directory."
    }
    if (Test-Path -LiteralPath $resolvedBuild) { Remove-Item -LiteralPath $resolvedBuild -Recurse -Force }
  } catch { $cleanupFailures += $_.Exception.Message }
  try { Pop-Location } catch { $cleanupFailures += $_.Exception.Message }
  if ($null -ne $failure) {
    if ($cleanupFailures.Count -gt 0) { throw "Windows service test failed: $($failure.Exception.Message) Cleanup also failed: $($cleanupFailures -join '; ')" }
    throw $failure
  }
  if ($cleanupFailures.Count -gt 0) { throw "Windows service cleanup failed: $($cleanupFailures -join '; ')" }
}

Write-Host "Windows SCM installation, file secrets, migration preflight, automatic start, Event Log, health, graceful stop, and restart passed."
