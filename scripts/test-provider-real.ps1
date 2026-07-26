$ErrorActionPreference = "Stop"

. "$PSScriptRoot\isolated-services.ps1"
. "$PSScriptRoot\acceptance\runtime.ps1"

function Invoke-ControlJSON {
  param(
    [Parameter(Mandatory = $true)][string] $Method,
    [Parameter(Mandatory = $true)][string] $Path,
    [Parameter(Mandatory = $true)] $Body,
    [switch] $Idempotent
  )

  $headers = @{ "X-CSRF-Token" = $script:AdminCSRF }
  if ($Idempotent) { $headers["Idempotency-Key"] = [guid]::NewGuid().ToString() }
  $encoded = $Body | ConvertTo-Json -Depth 10 -Compress
  return Invoke-RestMethod -Method $Method -Uri "$script:BaseURL$Path" -WebSession $script:AdminSession `
    -Headers $headers -ContentType "application/json" -Body $encoded -TimeoutSec 180
}

function New-ResourcePool {
  param(
    [Parameter(Mandatory = $true)][string] $ProviderID,
    [Parameter(Mandatory = $true)][string] $Name
  )

  return (Invoke-ControlJSON -Method Post -Path "/api/control/resource-pools" -Idempotent -Body @{
      providerId = $ProviderID; name = $Name
    }).data
}

function New-Credential {
  param(
    [Parameter(Mandatory = $true)][string] $ResourcePoolID,
    [Parameter(Mandatory = $true)][string] $Name,
    [Parameter(Mandatory = $true)][string] $Secret
  )

  $result = (Invoke-ControlJSON -Method Post -Path "/api/control/credentials/batch" -Idempotent -Body @{
      resourcePoolId = $ResourcePoolID
      items = @(@{ name = $Name; secret = $Secret })
      rpmLimit = 60
      tpmLimit = 1000000
      concurrencyLimit = 4
    }).data | Select-Object -First 1
  if ($result.status -ne "created" -or -not $result.credential.id) {
    throw "A real Provider upstream API Key was not created."
  }
  return $result.credential
}

function Invoke-SDKClient {
  param(
    [Parameter(Mandatory = $true)][ValidateSet("go", "python")][string] $SDK,
    [Parameter(Mandatory = $true)][string] $SuccessModel,
    [Parameter(Mandatory = $true)][string] $StreamModel,
    [Parameter(Mandatory = $true)][ValidateSet("toggle", "effort", "hybrid")][string] $ReasoningMode,
    [Parameter(Mandatory = $true)][string] $ErrorModel,
    [Parameter(Mandatory = $true)][string] $PythonPath
  )

  $env:LLM2API_SDK_BASE_URL = "$script:BaseURL/v1"
  $env:LLM2API_SDK_API_KEY = $script:GatewayKey
  $env:LLM2API_SDK_SUCCESS_MODEL = $SuccessModel
  $env:LLM2API_SDK_STREAM_MODEL = $StreamModel
  $env:LLM2API_SDK_REASONING_MODE = $ReasoningMode
  $env:LLM2API_SDK_ERROR_MODEL = $ErrorModel
  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    if ($SDK -eq "go") {
      $output = & go run . 2>$null
      $exitCode = $LASTEXITCODE
    } else {
      $output = & $PythonPath client.py 2>$null
      $exitCode = $LASTEXITCODE
    }
  } finally {
    $ErrorActionPreference = $previousPreference
    $env:LLM2API_SDK_API_KEY = $null
    $env:LLM2API_SDK_EXPLICIT_REISSUE = $null
    $env:LLM2API_SDK_STREAM_MODEL = $null
    $env:LLM2API_SDK_REASONING_MODE = $null
  }
  if (@($output).Count -ne 1) {
    throw "$SDK SDK acceptance failed without a valid redacted summary."
  }
  $summary = [string]($output | Select-Object -First 1) | ConvertFrom-Json
  if ($exitCode -ne 0 -or -not $summary.succeeded) {
    $failed = @($summary.scenarios | Where-Object { -not $_.succeeded } | ForEach-Object {
        "$($_.name):$($_.httpStatus):$($_.errorCode):$($_.errorType)"
      }) -join ","
    throw "$SDK SDK acceptance failed: $failed"
  }
  return $summary
}

function Invoke-ProviderCanary {
  param(
    [Parameter(Mandatory = $true)][string] $CanaryBinary,
    [Parameter(Mandatory = $true)][string] $Secret,
    [Parameter(Mandatory = $true)][string] $Kind,
    [Parameter(Mandatory = $true)][string] $ProviderBaseURL,
    [Parameter(Mandatory = $true)][string] $Model,
    [Parameter(Mandatory = $true)][string] $Scenarios
  )

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $output = $Secret | & $CanaryBinary -kind $Kind -base-url $ProviderBaseURL -model $Model `
      -scenarios $Scenarios -request-timeout 120s `
      -allowed-resolved-networks "198.18.0.0/15,fdfe:dcba:9876::/48" 2>$null
  } finally {
    $ErrorActionPreference = $previousPreference
  }
  if (@($output).Count -ne 1) { throw "Provider canary did not return one redacted summary." }
  return [string]($output | Select-Object -First 1) | ConvertFrom-Json
}

function Get-GatewayRetryDelaySeconds {
  param([Parameter(Mandatory = $true)] $Response)

  $value = [string]$Response.Headers["Retry-After"]
  if (-not $value) { return $null }
  $seconds = 0.0
  if ([double]::TryParse($value, [ref]$seconds)) {
    if ($seconds -lt 0 -or $seconds -gt 180) { return $null }
    return [Math]::Max($seconds + 0.25, 0.25)
  }
  $deadline = [DateTimeOffset]::MinValue
  if ([DateTimeOffset]::TryParse($value, [ref]$deadline)) {
    $delay = ($deadline.ToUniversalTime() - [DateTimeOffset]::UtcNow).TotalSeconds + 1.0
    if ($delay -ge 0 -and $delay -le 180) { return [Math]::Max($delay, 0.25) }
  }
  return $null
}

function Invoke-GatewayJSONWithExplicitReissue {
  param(
    [Parameter(Mandatory = $true)][string] $Path,
    [Parameter(Mandatory = $true)][string] $Body,
    [int] $MaxAttempts = 4
  )

  for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
    try {
      $data = Invoke-RestMethod -Method Post -Uri "$script:BaseURL$Path" `
        -Headers @{ Authorization = "Bearer $script:GatewayKey" } -ContentType "application/json" -Body $Body -TimeoutSec 180
      return [pscustomobject]@{ Succeeded = $true; Data = $data; HTTPStatus = 200; ErrorCode = "" }
    } catch {
      $response = $_.Exception.Response
      if ($null -eq $response) { throw }
      $status = [int]$response.StatusCode
      $code = ""
      try {
        $problem = $_.ErrorDetails.Message | ConvertFrom-Json
        $code = [string]$problem.error.code
      } catch {
        $code = ""
      }
      $retryable = $status -eq 429 -or
        ($status -eq 409 -and $code -eq "upstream_outcome_uncertain") -or
        ($status -eq 503 -and @("upstream_circuit_open", "503") -contains $code)
      $delaySeconds = Get-GatewayRetryDelaySeconds -Response $response
      if (-not $retryable -or $null -eq $delaySeconds) { throw }
      if ($attempt -eq $MaxAttempts) {
        return [pscustomobject]@{ Succeeded = $false; Data = $null; HTTPStatus = $status; ErrorCode = $code }
      }
      Start-Sleep -Milliseconds ([int][Math]::Ceiling($delaySeconds * 1000))
    }
  }
  throw "Explicit Gateway reissue ended without a result."
}

$root = Split-Path -Parent $PSScriptRoot
$runID = New-LLM2APITestRunID -Purpose "provider"
$buildDirectory = Join-Path $root ".build\provider-real-$runID"
$runningOnWindows = $env:OS -eq "Windows_NT"
$binaryName = if ($runningOnWindows) { "gateway.exe" } else { "gateway" }
$binaryPath = Join-Path $buildDirectory $binaryName
$canaryBinaryName = if ($runningOnWindows) { "providercanary.exe" } else { "providercanary" }
$canaryBinary = Join-Path $buildDirectory $canaryBinaryName
$stdoutPath = Join-Path $buildDirectory "gateway.stdout.log"
$stderrPath = Join-Path $buildDirectory "gateway.stderr.log"
$pythonEnvironment = Join-Path $buildDirectory "python"
$pythonPath = if ($runningOnWindows) { Join-Path $pythonEnvironment "Scripts\python.exe" } else { Join-Path $pythonEnvironment "bin/python" }
$environmentSnapshot = Save-LLM2APIEnvironment
$postgres = $null
$valkey = $null
$gatewayProcess = $null
$acceptancePassed = $false
$failure = $null
$cleanupFailures = [System.Collections.Generic.List[string]]::new()

Push-Location $root
try {
  Clear-LLM2APIEnvironment
  New-Item -ItemType Directory -Force $buildDirectory | Out-Null
  $zhipuLabelPrefix = [string]::Concat([char]0x667A, [char]0x8C31)
  $keyLines = @(Get-Content -Encoding UTF8 -LiteralPath (Join-Path $root "key.txt") | ForEach-Object { $_.Trim() } | Where-Object { $_ })
  $keyEntries = @($keyLines | ForEach-Object {
      $segments = @($_.Split([char]0xFF1A) | ForEach-Object { $_.Trim() })
      if ($segments[0] -notmatch '^agnes[1-3]$' -and $segments[0] -notmatch "^$zhipuLabelPrefix[1-3]$") {
        return
      }
      if ($segments.Count -lt 2 -or @($segments | Where-Object { -not $_ }).Count -ne 0) {
        throw "Each named real Provider credential must contain a label and a final secret segment."
      }
      [pscustomobject]@{ Label = $segments[0]; Secret = $segments[-1] }
    })
  $siliconKeys = @($keyLines | Where-Object { $_ -match '^sk-[A-Za-z0-9_-]{20,}$' })
  $keyLines = $null
  $keys = @($keyEntries | ForEach-Object { $_.Secret })
  $namedCredentialCount = @($keys).Count
  $siliconCredentialCount = @($siliconKeys).Count
  $shortCredentialCount = @((@($keys) + @($siliconKeys)) | Where-Object { $_.Length -lt 20 }).Count
  if ($namedCredentialCount -ne 6 -or $siliconCredentialCount -ne 1 -or $shortCredentialCount -ne 0) {
    throw "Real Provider acceptance credential counts are invalid (named=$namedCredentialCount, SiliconFlow=$siliconCredentialCount, short=$shortCredentialCount)."
  }

  & go build -trimpath -o $canaryBinary .\cmd\providercanary
  if ($LASTEXITCODE -ne 0) { throw "Could not build the Provider ownership canary." }
  $agnesKeys = [System.Collections.Generic.List[string]]::new()
  $zhipuKeys = [System.Collections.Generic.List[string]]::new()
  foreach ($entry in $keyEntries) {
    if ($entry.Label -match '^agnes[1-3]$') {
      $agnesKeys.Add($entry.Secret)
    } elseif ($entry.Label -match "^$zhipuLabelPrefix[1-3]$") {
      $zhipuKeys.Add($entry.Secret)
    } else {
      throw "A real Provider credential label is outside the expected six-label contract."
    }
  }
  if ($agnesKeys.Count -ne 3 -or $zhipuKeys.Count -ne 3) {
    throw "Credential labels did not classify three Agnes and three Zhipu credentials."
  }
  foreach ($secret in $agnesKeys) {
    $probe = Invoke-ProviderCanary -CanaryBinary $canaryBinary -Secret $secret -Kind "agnes" `
      -ProviderBaseURL "https://apihub.agnes-ai.com/v1" -Model "agnes-2.0-flash" -Scenarios "models"
    if (-not $probe.succeeded) { throw "An Agnes credential failed its official models probe." }
  }
  foreach ($secret in $zhipuKeys) {
    $probe = Invoke-ProviderCanary -CanaryBinary $canaryBinary -Secret $secret -Kind "zhipu" `
      -ProviderBaseURL "https://open.bigmodel.cn/api/paas/v4" -Model "glm-5.2" -Scenarios "models"
    if (-not $probe.succeeded) { throw "A Zhipu credential failed its official models probe." }
  }
  $siliconProbe = Invoke-ProviderCanary -CanaryBinary $canaryBinary -Secret $siliconKeys[0] -Kind "siliconflow" `
    -ProviderBaseURL "https://api.siliconflow.cn/v1" -Model "Qwen/Qwen3.5-9B" -Scenarios "models"
  if (-not $siliconProbe.succeeded) { throw "The SiliconFlow credential failed its official models probe." }

  $zhipuQuotaKeys = [System.Collections.Generic.List[string]]::new()
  $zhipuSuccessKeys = [System.Collections.Generic.List[string]]::new()
  foreach ($secret in $zhipuKeys) {
    $chatProbe = Invoke-ProviderCanary -CanaryBinary $canaryBinary -Secret $secret -Kind "zhipu" `
      -ProviderBaseURL "https://open.bigmodel.cn/api/paas/v4" -Model "glm-5.2" -Scenarios "chat"
    $scenario = @($chatProbe.scenarios | Select-Object -First 1)
    if ($chatProbe.succeeded) {
      $zhipuSuccessKeys.Add($secret)
    } elseif ($scenario.Count -eq 1 -and $scenario[0].errorKind -eq "quota") {
      $zhipuQuotaKeys.Add($secret)
    } else {
      throw "A Zhipu test credential returned neither success nor the expected quota fact."
    }
  }
  if ($zhipuQuotaKeys.Count -ne 2 -or $zhipuSuccessKeys.Count -ne 1) {
    throw "Zhipu canary did not confirm two quota credentials and one healthy credential."
  }

  $postgres = Start-LLM2APITestPostgres -RunID $runID -DatabaseName "llm2api_provider" -Password "provider-postgres-fixture"
  $valkeyPassword = "provider-valkey-fixture"
  $valkey = Start-LLM2APITestValkey -RunID $runID -Password $valkeyPassword
  $gatewayPort = Get-AcceptanceLoopbackPort
  $script:BaseURL = "http://127.0.0.1:$gatewayPort"

  $env:LLM2API_PROFILE = "test"
  $env:LLM2API_HTTP_ADDRESS = "127.0.0.1:$gatewayPort"
  $env:LLM2API_HTTP_IDLE_TIMEOUT = "180s"
  $env:LLM2API_DATABASE_URL = $postgres.DatabaseURL
  $env:LLM2API_DATABASE_MIGRATE_ON_START = "true"
  $env:LLM2API_VALKEY_ADDRESS = $valkey.Address
  $env:LLM2API_VALKEY_PASSWORD = $valkeyPassword
  $env:LLM2API_VALKEY_DATABASE = "0"
  $env:LLM2API_MASTER_KEYS = "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
  $env:LLM2API_ACTIVE_MASTER_KEY_VERSION = "1"
  $env:LLM2API_SESSION_PEPPER = "llm2api-provider-session-pepper-0000"
  $env:LLM2API_API_KEY_PEPPER = "llm2api-provider-api-key-pepper-0000"
  $env:LLM2API_CREDENTIAL_FINGERPRINT_PEPPER = "llm2api-provider-credential-fingerprint-pepper"
  $env:LLM2API_COORDINATION_KEY_HASH_SECRET = "llm2api-provider-coordination-pepper"
  $env:LLM2API_COOKIE_SECURE = "false"
  $env:LLM2API_ALLOWED_RESOLVED_NETWORKS = "198.18.0.0/15,fdfe:dcba:9876::/48"
  $env:LLM2API_PROVIDER_PROBE_TIMEOUT = "120s"
  $env:LLM2API_REQUEST_MAX_QUEUE_WAIT = "150s"
  $env:LLM2API_REQUEST_RETRY_MAX_ELAPSED = "120s"
  $env:LLM2API_LOG_LEVEL = "info"

  & pnpm.cmd --dir web run build
  if ($LASTEXITCODE -ne 0) { throw "Could not build the production frontend for real Provider acceptance." }
  & go build -tags webembed -trimpath -o $binaryPath .\cmd\gateway
  if ($LASTEXITCODE -ne 0) { throw "Could not build the real Provider gateway." }
  $gatewayProcess = Start-AcceptanceProcess -BinaryPath $binaryPath -StandardOutputPath $stdoutPath -StandardErrorPath $stderrPath
  Wait-AcceptanceReadiness -Process $gatewayProcess -ReadinessURL "$script:BaseURL/health/ready" -TimeoutSeconds 60

  $script:AdminSession = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $setup = Invoke-RestMethod -Method Post -Uri "$script:BaseURL/api/control/setup" -WebSession $script:AdminSession `
    -ContentType "application/json" -Body (@{
      email = "provider-owner@example.test"
    } | ConvertTo-Json) -TimeoutSec 30
  if ($setup.data.role -ne "administrator" -or -not $setup.data.csrfToken -or -not $setup.data.initialPassword) {
    throw "Real Provider acceptance could not establish the administrator."
  }
  $script:AdminCSRF = $setup.data.csrfToken
  $setup.data.initialPassword = $null

  $providerCatalog = Invoke-RestMethod -Uri "$script:BaseURL/api/control/providers" -WebSession $script:AdminSession -TimeoutSec 30
  $agnes = @($providerCatalog.data | Where-Object { $_.catalog_id -eq "agnes" }) | Select-Object -First 1
  $zhipu = @($providerCatalog.data | Where-Object { $_.catalog_id -eq "zhipu" }) | Select-Object -First 1
  $silicon = @($providerCatalog.data | Where-Object { $_.catalog_id -eq "siliconflow" }) | Select-Object -First 1
  if (@(@($agnes, $zhipu, $silicon) | Where-Object { $null -eq $_ }).Count -gt 0) {
    throw "The code-owned Provider catalog did not expose the three verified real entry points."
  }

  $agnesPool = New-ResourcePool -ProviderID $agnes.id -Name "Real Agnes"
  $zhipuPool = New-ResourcePool -ProviderID $zhipu.id -Name "Real Zhipu"
  $siliconPool = New-ResourcePool -ProviderID $silicon.id -Name "Real SiliconFlow"
  if (@(@($agnesPool, $zhipuPool, $siliconPool) | Where-Object { $_.status -ne "active" }).Count -gt 0) {
    throw "A real Provider resource pool did not become active."
  }

  for ($index = 0; $index -lt 3; $index++) {
    New-Credential -ResourcePoolID $agnesPool.id -Name "Agnes dedicated $($index + 1)" -Secret $agnesKeys[$index] `
      | Out-Null
  }
  New-Credential -ResourcePoolID $zhipuPool.id -Name "Zhipu quota 1" -Secret $zhipuQuotaKeys[0] | Out-Null
  New-Credential -ResourcePoolID $zhipuPool.id -Name "Zhipu success" -Secret $zhipuSuccessKeys[0] | Out-Null
  New-Credential -ResourcePoolID $zhipuPool.id -Name "Zhipu quota 3" -Secret $zhipuQuotaKeys[1] | Out-Null
  New-Credential -ResourcePoolID $siliconPool.id -Name "SiliconFlow dedicated 1" -Secret $siliconKeys[0] | Out-Null

  $modelCatalog = Invoke-RestMethod -Uri "$script:BaseURL/api/control/models" -WebSession $script:AdminSession -TimeoutSec 30
  $agnesModel = @($modelCatalog.data | Where-Object { $_.provider_id -eq $agnes.id -and $_.public_name -eq "agnes-2.0-flash" }) | Select-Object -First 1
  $zhipuModel = @($modelCatalog.data | Where-Object { $_.provider_id -eq $zhipu.id -and $_.public_name -eq "glm-5.2" }) | Select-Object -First 1
  $siliconModel = @($modelCatalog.data | Where-Object { $_.provider_id -eq $silicon.id -and $_.public_name -eq "Qwen/Qwen3.5-9B" }) | Select-Object -First 1
  if (@(@($agnesModel, $zhipuModel, $siliconModel) | Where-Object { $null -eq $_ }).Count -gt 0) {
    throw "Upstream model discovery did not expose the three verified model contracts after credential import."
  }

  $plan = Invoke-ControlJSON -Method Post -Path "/api/control/plans" -Idempotent -Body @{
    name = "Real Provider Plan"
    description = "Isolated real Provider acceptance"
    routes = @(
      @{ modelId = $agnesModel.id; resourcePoolId = $agnesPool.id },
      @{ modelId = $zhipuModel.id; resourcePoolId = $zhipuPool.id },
      @{ modelId = $siliconModel.id; resourcePoolId = $siliconPool.id }
    )
  }
  if ($plan.data.current_version.version -ne 1 -or @($plan.data.current_version.routes).Count -ne 3) {
    throw "Real Provider plan publication did not freeze the three catalog routes."
  }
  $subscription = Invoke-ControlJSON -Method Post -Path "/api/control/subscriptions" -Idempotent -Body @{
    userId = $setup.data.userId
    servicePlanId = $plan.data.id
    startsAt = (Get-Date).ToUniversalTime().AddMinutes(-1).ToString("o")
    expiresAt = (Get-Date).ToUniversalTime().AddDays(1).ToString("o")
    notes = "Real Provider acceptance"
  }
  if ($subscription.data.status -ne "active" -or $subscription.data.service_plan_version_id -ne $plan.data.current_version.id) {
    throw "Real Provider subscription did not freeze the published plan version."
  }
  $keyResult = Invoke-ControlJSON -Method Post -Path "/api/control/keys" -Idempotent -Body @{
    ownerId = $setup.data.userId
    name = "Real Provider SDK"
    routes = @(
      @{ modelId = $agnesModel.id; resourcePoolId = $agnesPool.id }
      @{ modelId = $zhipuModel.id; resourcePoolId = $zhipuPool.id }
      @{ modelId = $siliconModel.id; resourcePoolId = $siliconPool.id }
    )
    expiresAt = $null
  }
  $script:GatewayKey = $keyResult.data.secret
  if (-not $script:GatewayKey -or -not $script:GatewayKey.StartsWith("llmg_")) {
    throw "Real Provider API key was not issued."
  }

  Push-Location (Join-Path $root "scripts\acceptance\openai-go")
  try {
    $goSummary = Invoke-SDKClient -SDK go -SuccessModel $siliconModel.public_name -StreamModel $siliconModel.public_name `
      -ReasoningMode "toggle" -ErrorModel $zhipuModel.public_name -PythonPath $pythonPath
  } finally {
    Pop-Location
  }

  & python -m venv $pythonEnvironment
  if ($LASTEXITCODE -ne 0) { throw "Could not create the isolated Python SDK environment." }
  & $pythonPath -m pip install --disable-pip-version-check --requirement (Join-Path $root "scripts\acceptance\openai-python\requirements.txt") *> $null
  if ($LASTEXITCODE -ne 0) { throw "Could not install the pinned Python SDK." }
  Push-Location (Join-Path $root "scripts\acceptance\openai-python")
  try {
    $env:LLM2API_SDK_EXPLICIT_REISSUE = "true"
    $pythonSummary = Invoke-SDKClient -SDK python -SuccessModel $siliconModel.public_name -StreamModel $agnesModel.public_name `
      -ReasoningMode "toggle" -ErrorModel $zhipuModel.public_name -PythonPath $pythonPath
  } finally {
    Pop-Location
  }

  $dedicatedToolBody = @{
    model = $agnesModel.public_name
    messages = @(@{ role = "user"; content = "Call lookup for Beijing. Do not answer directly." })
    tools = @(@{ type = "function"; function = @{
          name = "lookup"; description = "Look up a city"
          parameters = @{ type = "object"; properties = @{ city = @{ type = "string" } }; required = @("city") }
        } })
    max_tokens = 64
  } | ConvertTo-Json -Depth 10 -Compress
  $dedicatedToolResult = Invoke-GatewayJSONWithExplicitReissue -Path "/v1/chat/completions" -Body $dedicatedToolBody
  if (-not $dedicatedToolResult.Succeeded) {
    throw "The dedicated Agnes tool request remained unavailable after bounded explicit reissue."
  }
  $dedicatedTool = $dedicatedToolResult.Data
  if (@($dedicatedTool.choices).Count -eq 0 -or @($dedicatedTool.choices[0].message.tool_calls).Count -eq 0) {
    throw "The dedicated Agnes adapter did not return an automatic tool call through the Gateway."
  }

  $dedicatedReasoningBody = @{
    model = $agnesModel.public_name
    messages = @(@{ role = "user"; content = "Reply with exactly OK after thinking." })
    thinking = @{ type = "enabled" }
    max_tokens = 64
  } | ConvertTo-Json -Depth 6 -Compress
  $dedicatedReasoningResult = Invoke-GatewayJSONWithExplicitReissue -Path "/v1/chat/completions" -Body $dedicatedReasoningBody
  if (-not $dedicatedReasoningResult.Succeeded) {
    throw "The dedicated Agnes thinking request remained unavailable after bounded explicit reissue."
  }
  $dedicatedReasoning = $dedicatedReasoningResult.Data
  if (@($dedicatedReasoning.choices).Count -eq 0 -or $dedicatedReasoning.usage.total_tokens -lt 1) {
    throw "The dedicated Agnes thinking request did not complete with usage through the Gateway."
  }

  $chatBody = @{
    model = $zhipuModel.public_name
    messages = @(@{ role = "user"; content = "Reply with exactly OK." })
    max_tokens = 32
  } | ConvertTo-Json -Depth 5 -Compress
  $zhipuSuccess = Invoke-RestMethod -Method Post -Uri "$script:BaseURL/v1/chat/completions" `
    -Headers @{ Authorization = "Bearer $script:GatewayKey" } -ContentType "application/json" -Body $chatBody -TimeoutSec 150
  if (-not $zhipuSuccess.id -or @($zhipuSuccess.choices).Count -eq 0 -or $zhipuSuccess.usage.total_tokens -lt 1) {
    throw "The remaining healthy Zhipu credential did not take over with authoritative usage."
  }

  $docker = Get-LLM2APIDockerCommand
  $zhipuFacts = & $docker exec $postgres.Container psql -v ON_ERROR_STOP=1 -U llm2api -d llm2api_provider -Atc `
    "SELECT string_agg(credential.name || ':' || credential.status::text || ':' || coalesce(credential.last_error_kind, 'ok'), ',' ORDER BY credential.name) FROM provider_credentials credential JOIN resource_pools pool ON pool.id = credential.resource_pool_id WHERE pool.provider_id = '$($zhipu.id)'"
  if ($LASTEXITCODE -ne 0 -or $zhipuFacts -ne "Zhipu quota 1:cooling:quota,Zhipu quota 3:cooling:quota,Zhipu success:active:ok") {
    throw "Real Zhipu quota exclusion and healthy credential takeover did not persist."
  }
  foreach ($secret in $keys) {
    if (Select-String -LiteralPath @($stdoutPath, $stderrPath) -SimpleMatch -Quiet -Pattern $secret) {
      throw "A real Provider credential appeared in a Gateway runtime log."
    }
  }
  if (@($goSummary.scenarios).Count -ne 8 -or @($pythonSummary.scenarios).Count -ne 7) {
    throw "The standard SDK summaries did not cover the complete scenario set."
  }
  $acceptancePassed = $true
} catch {
  $failure = $_
} finally {
  $script:GatewayKey = $null
  $keys = $null
  $keyEntries = $null
  $agnesKeys = $null
  $zhipuKeys = $null
  $siliconKeys = $null
  $zhipuQuotaKeys = $null
  $zhipuSuccessKeys = $null
  try { Stop-AcceptanceProcess -Process $gatewayProcess -ExpectedBinaryPath $binaryPath } catch { $cleanupFailures.Add($_.Exception.Message) }
  if ($null -ne $valkey) {
    try { Stop-LLM2APITestContainer -Container $valkey.Container -RunID $runID } catch { $cleanupFailures.Add($_.Exception.Message) }
  }
  if ($null -ne $postgres) {
    try { Stop-LLM2APITestContainer -Container $postgres.Container -RunID $runID } catch { $cleanupFailures.Add($_.Exception.Message) }
  }
  Restore-LLM2APIEnvironment -Snapshot $environmentSnapshot
  Pop-Location
  if ($acceptancePassed -and $cleanupFailures.Count -eq 0) {
    try { Remove-AcceptanceBuildDirectory -RepositoryRoot $root -BuildDirectory $buildDirectory -ExpectedPrefix "provider-real-" } catch { $cleanupFailures.Add($_.Exception.Message) }
  }
}

if ($null -ne $failure) {
  if ($cleanupFailures.Count -gt 0) { throw "$($failure.Exception.Message) Cleanup: $($cleanupFailures -join '; ')" }
  throw $failure
}
if ($cleanupFailures.Count -gt 0) { throw "Real Provider acceptance cleanup failed: $($cleanupFailures -join '; ')" }
if (-not $acceptancePassed) { throw "Real Provider acceptance ended without a result." }

Write-Host "Real Agnes, Zhipu, SiliconFlow, Go SDK, Python SDK, quota exclusion, and healthy takeover acceptance passed."
