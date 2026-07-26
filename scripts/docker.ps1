$ErrorActionPreference = "Stop"

function Get-LLM2APIDockerCommand {
  $command = Get-Command docker -ErrorAction SilentlyContinue
  if ($command) {
    return $command.Source
  }

  $dockerDesktopCommand = "C:\Program Files\Docker\Docker\resources\bin\docker.exe"
  if (Test-Path -LiteralPath $dockerDesktopCommand) {
    return $dockerDesktopCommand
  }

  throw "Docker CLI was not found. Start Docker Desktop and reopen the terminal."
}

function Invoke-LLM2APIDocker {
  param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $Arguments
  )

  $docker = Get-LLM2APIDockerCommand
  & $docker @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "Docker command failed: docker $($Arguments -join ' ')"
  }
}

function Get-LLM2APIAcceptanceNetworkAllocation {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Docker,
    [Parameter(Mandatory = $true)]
    [string] $RunID
  )

  if ($RunID -notmatch '^[a-z][a-z0-9-]{1,63}$') {
    throw "Refusing to allocate Docker networks for an invalid test-run ID."
  }

  $owner = "acceptance-network-probe"
  $backendProbeName = "llm2api-backend-probe-$RunID"
  $edgeProbeName = "llm2api-edge-probe-$RunID"
  $backendCreated = $false
  $edgeCreated = $false

  try {
    & $Docker network create --internal `
      --label "llm2api.test.owner=$owner" `
      --label "llm2api.test.run=$RunID" $backendProbeName *> $null
    if ($LASTEXITCODE -ne 0) {
      throw "Docker could not allocate an isolated acceptance backend network."
    }
    $backendCreated = $true

    & $Docker network create --internal `
      --label "llm2api.test.owner=$owner" `
      --label "llm2api.test.run=$RunID" $edgeProbeName *> $null
    if ($LASTEXITCODE -ne 0) {
      throw "Docker could not allocate an isolated acceptance edge network."
    }
    $edgeCreated = $true

    $backendJSON = ((@(& $Docker network inspect $backendProbeName 2>$null) -join "`n") | ConvertFrom-Json | Select-Object -First 1)
    $backendExitCode = $LASTEXITCODE
    $edgeJSON = ((@(& $Docker network inspect $edgeProbeName 2>$null) -join "`n") | ConvertFrom-Json | Select-Object -First 1)
    $edgeExitCode = $LASTEXITCODE
    $backendConfiguration = if ($null -ne $backendJSON) { @($backendJSON.IPAM.Config | Select-Object -First 1)[0] } else { $null }
    $edgeConfiguration = if ($null -ne $edgeJSON) { @($edgeJSON.IPAM.Config | Select-Object -First 1)[0] } else { $null }
    $backendSubnet = if ($null -ne $backendConfiguration) { [string]$backendConfiguration.Subnet } else { "" }
    $edgeSubnet = if ($null -ne $edgeConfiguration) { [string]$edgeConfiguration.Subnet } else { "" }
    $edgeGateway = if ($null -ne $edgeConfiguration) { [string]$edgeConfiguration.Gateway } else { "" }
    $edgeSubnetMatch = [regex]::Match($edgeSubnet, '^(?<prefix>\d+\.\d+\.\d+)\.0/\d+$')
    if ($backendExitCode -ne 0 -or $edgeExitCode -ne 0 -or $null -eq $backendJSON -or $null -eq $edgeJSON -or
        $backendJSON.Labels.'llm2api.test.owner' -ne $owner -or
        $edgeJSON.Labels.'llm2api.test.owner' -ne $owner -or
        $backendJSON.Labels.'llm2api.test.run' -ne $RunID -or
        $edgeJSON.Labels.'llm2api.test.run' -ne $RunID -or
        $backendSubnet -eq $edgeSubnet -or
        -not $edgeSubnetMatch.Success -or
        $edgeGateway -notmatch '^\d+\.\d+\.\d+\.\d+$') {
      throw "Docker returned an invalid isolated acceptance network allocation."
    }

    return [pscustomobject]@{
      BackendSubnet = $backendSubnet
      EdgeSubnet    = $edgeSubnet
      ProxyAddress  = "$($edgeSubnetMatch.Groups['prefix'].Value).10"
    }
  } finally {
    foreach ($probeName in @($edgeProbeName, $backendProbeName)) {
      if (($probeName -eq $edgeProbeName -and -not $edgeCreated) -or ($probeName -eq $backendProbeName -and -not $backendCreated)) {
        continue
      }
      & $Docker network rm $probeName *> $null
      if ($LASTEXITCODE -ne 0) {
        throw "Could not remove the acceptance network probe $probeName."
      }
    }
  }
}

function Wait-LLM2APIContainerHealthy {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Container,
    [int] $TimeoutSeconds = 180
  )

  $docker = Get-LLM2APIDockerCommand
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

  do {
    $status = & $docker inspect --format "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $Container 2>$null
    if ($LASTEXITCODE -eq 0) {
      if ($status -eq "healthy") {
        return
      }
      if ($status -eq "unhealthy" -or $status -eq "exited" -or $status -eq "dead") {
        throw "$Container entered terminal state: $status"
      }
    }

    Start-Sleep -Milliseconds 500
  } while ((Get-Date) -lt $deadline)

  throw "Timed out waiting for $Container to become healthy."
}

function Test-LLM2APIPostgres {
  $docker = Get-LLM2APIDockerCommand
  & $docker exec llm2api-postgres sh -c 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' | Out-Null
  if ($LASTEXITCODE -ne 0) {
    throw "PostgreSQL did not accept an authenticated readiness check."
  }
}

function Test-LLM2APIValkey {
  $docker = Get-LLM2APIDockerCommand
  $response = & $docker exec llm2api-valkey sh -c 'valkey-cli --no-auth-warning -a "$VALKEY_PASSWORD" ping'
  if ($LASTEXITCODE -ne 0 -or $response -ne "PONG") {
    throw "Valkey did not accept an authenticated PING."
  }
}
