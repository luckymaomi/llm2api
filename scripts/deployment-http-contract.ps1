$DeploymentAdministratorEmail = "deployment-admin@example.test"
$DeploymentAdministratorPassword = "deployment-administrator-password"
$DeploymentMemberEmail = "deployment-member@example.test"
$DeploymentMemberPassword = "deployment-member-password"

function New-DeploymentHTTPClient([string] $BaseURL) {
  $cookieJar = [IO.Path]::GetTempFileName()
  return [pscustomobject]@{ BaseURL = $BaseURL.TrimEnd('/'); CookieJar = $cookieJar }
}

function Remove-DeploymentHTTPClient($Client) {
  Remove-Item -LiteralPath $Client.CookieJar -Force -ErrorAction SilentlyContinue
}

function Invoke-DeploymentHTTPRequest {
  param(
    [Parameter(Mandatory = $true)]$Client,
    [Parameter(Mandatory = $true)][string] $Method,
    [Parameter(Mandatory = $true)][string] $Path,
    [int] $ExpectedStatus = 200,
    [object] $Body,
    [string] $CSRF,
    [string] $IdempotencyKey,
    [switch] $RawContent
  )
  $responsePath = [IO.Path]::GetTempFileName()
  $bodyPath = $null
  try {
    $arguments = @(
      "--insecure", "--silent", "--show-error", "--request", $Method,
      "--cookie", $Client.CookieJar, "--cookie-jar", $Client.CookieJar,
      "--header", "Accept: application/json", "--output", $responsePath,
      "--write-out", "%{http_code}"
    )
    if ($CSRF) { $arguments += @("--header", "X-CSRF-Token: $CSRF") }
    if ($IdempotencyKey) { $arguments += @("--header", "Idempotency-Key: $IdempotencyKey") }
    if ($null -ne $Body) {
      $json = $Body | ConvertTo-Json -Depth 12 -Compress
      $bodyPath = [IO.Path]::GetTempFileName()
      [IO.File]::WriteAllText($bodyPath, $json, [Text.UTF8Encoding]::new($false))
      $arguments += @("--header", "Content-Type: application/json", "--data-binary", "@$bodyPath")
    }
    $status = (& curl.exe @arguments "$($Client.BaseURL)$Path").Trim()
    if ($LASTEXITCODE -ne 0 -or $status -notmatch '^[0-9]{3}$') {
      throw "$Method $Path could not reach the deployment HTTP entry."
    }
    $content = [IO.File]::ReadAllText($responsePath, [Text.Encoding]::UTF8)
    if ([int]$status -ne $ExpectedStatus) {
      $summary = $content.Replace("`r", " ").Replace("`n", " ").Trim()
      if ($summary.Length -gt 500) { $summary = $summary.Substring(0, 500) }
      throw "$Method $Path returned $status, expected ${ExpectedStatus}: $summary"
    }
    if ($RawContent) { return $content }
    if ([string]::IsNullOrWhiteSpace($content)) { return $null }
    return $content | ConvertFrom-Json
  } finally {
    Remove-Item -LiteralPath $responsePath -Force -ErrorAction SilentlyContinue
    if ($bodyPath) { Remove-Item -LiteralPath $bodyPath -Force -ErrorAction SilentlyContinue }
  }
}

function Assert-DeploymentFrontend {
  param(
    [Parameter(Mandatory = $true)]$Client
  )
  $html = Invoke-DeploymentHTTPRequest -Client $Client -Method "GET" -Path "/" -RawContent
  if ($html -notmatch '<div id="root"></div>' -or $html -notmatch '/assets/[^"'']+\.js') {
    throw "The production entry did not serve the built frontend shell."
  }
  $assetPath = [regex]::Match($html, '/assets/[^"'']+\.js').Value
  $asset = Invoke-DeploymentHTTPRequest -Client $Client -Method "GET" -Path $assetPath -RawContent
  if ([string]::IsNullOrWhiteSpace($asset)) { throw "The production entry did not serve the frontend JavaScript asset." }
}

function Invoke-DeploymentHTTPContract {
  param(
    [Parameter(Mandatory = $true)][ValidateSet("setup", "restored")][string] $Mode,
    [Parameter(Mandatory = $true)][string] $BaseURL
  )
  $admin = New-DeploymentHTTPClient $BaseURL
  try {
    Assert-DeploymentFrontend -Client $admin
    if ($Mode -eq "setup") {
      $setup = Invoke-DeploymentHTTPRequest -Client $admin -Method "POST" -Path "/api/control/setup" -ExpectedStatus 201 -Body @{
        email = $DeploymentAdministratorEmail
      }
      $initialAdministratorPassword = [string]$setup.data.initialPassword
      $adminCSRF = [string]$setup.data.csrfToken
      if ([string]::IsNullOrWhiteSpace($initialAdministratorPassword) -or [string]::IsNullOrWhiteSpace($adminCSRF)) {
        throw "Administrator setup did not return one-time credentials and CSRF state."
      }
      $null = Invoke-DeploymentHTTPRequest -Client $admin -Method "POST" -Path "/api/control/password" -Body @{
        currentPassword = $initialAdministratorPassword
        replacementPassword = $DeploymentAdministratorPassword
      } -CSRF $adminCSRF
      $member = Invoke-DeploymentHTTPRequest -Client $admin -Method "POST" -Path "/api/control/members" -ExpectedStatus 201 -Body @{
        email = $DeploymentMemberEmail
        displayName = "Deployment Member"
      } -CSRF $adminCSRF -IdempotencyKey ([guid]::NewGuid().ToString())
      $initialMemberPassword = [string]$member.data.initialPassword
      if ([string]::IsNullOrWhiteSpace($initialMemberPassword)) {
        throw "Member creation did not return a one-time password."
      }
      $memberClient = New-DeploymentHTTPClient $BaseURL
      try {
        $memberSession = Invoke-DeploymentHTTPRequest -Client $memberClient -Method "POST" -Path "/api/control/session" -Body @{
          email = $DeploymentMemberEmail
          password = $initialMemberPassword
        }
        $null = Invoke-DeploymentHTTPRequest -Client $memberClient -Method "POST" -Path "/api/control/password" -Body @{
          currentPassword = $initialMemberPassword
          replacementPassword = $DeploymentMemberPassword
        } -CSRF ([string]$memberSession.data.csrfToken)
        $null = Invoke-DeploymentHTTPRequest -Client $memberClient -Method "GET" -Path "/api/control/members" -ExpectedStatus 403
      } finally {
        Remove-DeploymentHTTPClient $memberClient
      }
      return
    }

    $adminSession = Invoke-DeploymentHTTPRequest -Client $admin -Method "POST" -Path "/api/control/session" -Body @{
      email = $DeploymentAdministratorEmail
      password = $DeploymentAdministratorPassword
    }
    if ($adminSession.data.role -ne "administrator") { throw "The restored administrator could not authenticate." }
    $memberClient = New-DeploymentHTTPClient $BaseURL
    try {
      $memberSession = Invoke-DeploymentHTTPRequest -Client $memberClient -Method "POST" -Path "/api/control/session" -Body @{
        email = $DeploymentMemberEmail
        password = $DeploymentMemberPassword
      }
      if ($memberSession.data.role -ne "member") { throw "The restored member could not authenticate." }
      $null = Invoke-DeploymentHTTPRequest -Client $memberClient -Method "GET" -Path "/api/control/members" -ExpectedStatus 403
    } finally {
      Remove-DeploymentHTTPClient $memberClient
    }
  } finally {
    Remove-DeploymentHTTPClient $admin
  }
}
