$ErrorActionPreference = "Stop"

. "$PSScriptRoot\docker.ps1"

$script:LLM2APITestOwner = "llm2api-isolated-tests"

function Invoke-LLM2APINativeProbe {
  param([Parameter(Mandatory = $true)][scriptblock] $Action)

  $previousPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $output = & $Action 2>$null
    $exitCode = $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $previousPreference
  }
  return [pscustomobject]@{ ExitCode = $exitCode; Output = @($output) }
}

function New-LLM2APITestRunID {
  param([Parameter(Mandatory = $true)][string] $Purpose)

  if ($Purpose -notmatch '^[a-z][a-z0-9-]{0,19}$') {
    throw "Test purpose must be a short lowercase container-name segment."
  }
  $suffix = [guid]::NewGuid().ToString("N").Substring(0, 8)
  return "$Purpose-$PID-$suffix".ToLowerInvariant()
}

function Get-LLM2APIFreeLoopbackPort {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  try {
    $listener.Start()
    return ([System.Net.IPEndPoint] $listener.LocalEndpoint).Port
  } finally {
    $listener.Stop()
  }
}

function Get-LLM2APIPublishedPort {
  param(
    [Parameter(Mandatory = $true)][string] $Container,
    [Parameter(Mandatory = $true)][string] $Target
  )

  $docker = Get-LLM2APIDockerCommand
  $binding = & $docker port $Container $Target
  if ($LASTEXITCODE -ne 0) {
    throw "Could not read the published port for isolated container $Container."
  }
  $match = [regex]::Match(($binding | Select-Object -First 1), ':(\d+)$')
  if (-not $match.Success) {
    throw "Docker returned an invalid port binding for isolated container $Container."
  }
  return [int] $match.Groups[1].Value
}

function Wait-LLM2APIPostgresReady {
  param(
    [Parameter(Mandatory = $true)][string] $Container,
    [Parameter(Mandatory = $true)][string] $DatabaseName,
    [Parameter(Mandatory = $true)][string] $Password
  )

  $docker = Get-LLM2APIDockerCommand
  $deadline = (Get-Date).AddSeconds(60)
  do {
    $probe = Invoke-LLM2APINativeProbe {
      & $docker exec --env "PGPASSWORD=$Password" $Container `
        psql -h 127.0.0.1 -U llm2api -d $DatabaseName -Atc "SELECT 1"
    }
    if ($probe.ExitCode -eq 0 -and $probe.Output.Count -eq 1 -and $probe.Output[0] -eq "1") {
      return
    }
    $state = Invoke-LLM2APINativeProbe { & $docker inspect --format '{{.State.Running}}' $Container }
    if ($state.ExitCode -ne 0 -or $state.Output.Count -ne 1 -or $state.Output[0] -ne "true") {
      throw "The isolated PostgreSQL container stopped before readiness."
    }
    Start-Sleep -Milliseconds 200
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for isolated PostgreSQL."
}

function Wait-LLM2APIValkeyReady {
  param(
    [Parameter(Mandatory = $true)][string] $Container,
    [Parameter(Mandatory = $true)][string] $Password
  )

  $docker = Get-LLM2APIDockerCommand
  $deadline = (Get-Date).AddSeconds(60)
  do {
    $probe = Invoke-LLM2APINativeProbe { & $docker exec $Container valkey-cli --no-auth-warning -a $Password ping }
    if ($probe.ExitCode -eq 0 -and $probe.Output.Count -eq 1 -and $probe.Output[0] -eq "PONG") {
      return
    }
    $state = Invoke-LLM2APINativeProbe { & $docker inspect --format '{{.State.Running}}' $Container }
    if ($state.ExitCode -ne 0 -or $state.Output.Count -ne 1 -or $state.Output[0] -ne "true") {
      throw "The isolated Valkey container stopped before readiness."
    }
    Start-Sleep -Milliseconds 200
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for isolated Valkey."
}

function Start-LLM2APITestPostgres {
  param(
    [Parameter(Mandatory = $true)][string] $RunID,
    [Parameter(Mandatory = $true)][string] $DatabaseName,
    [Parameter(Mandatory = $true)][string] $Password
  )

  $docker = Get-LLM2APIDockerCommand
  $containerName = "llm2api-test-postgres-$RunID"
  $containerOutput = & $docker run --detach --rm --name $containerName `
    --label "llm2api.test.owner=$script:LLM2APITestOwner" `
    --label "llm2api.test.run=$RunID" `
    --publish "127.0.0.1::5432" `
    --env "POSTGRES_DB=$DatabaseName" `
    --env "POSTGRES_USER=llm2api" `
    --env "POSTGRES_PASSWORD=$Password" `
    postgres:18.4-alpine
  $containerExitCode = $LASTEXITCODE
  $container = [string] ($containerOutput | Select-Object -First 1)
  $container = $container.Trim()
  if ($containerExitCode -ne 0 -or -not $container) {
    throw "Could not start isolated PostgreSQL."
  }

  try {
    Wait-LLM2APIPostgresReady -Container $container -DatabaseName $DatabaseName -Password $Password
    $port = Get-LLM2APIPublishedPort -Container $container -Target "5432/tcp"
    return [pscustomobject]@{
      Container   = $container
      DatabaseURL = "postgres://llm2api:$Password@127.0.0.1:$port/$DatabaseName`?sslmode=disable"
    }
  } catch {
    $startFailure = $_
    try {
      Stop-LLM2APITestContainer -Container $container -RunID $RunID
    } catch {
      throw "PostgreSQL startup failed: $($startFailure.Exception.Message) Cleanup also failed: $($_.Exception.Message)"
    }
    throw $startFailure
  }
}

function Start-LLM2APITestValkey {
  param(
    [Parameter(Mandatory = $true)][string] $RunID,
    [Parameter(Mandatory = $true)][string] $Password
  )

  $docker = Get-LLM2APIDockerCommand
  $containerName = "llm2api-test-valkey-$RunID"
  $containerOutput = & $docker run --detach --rm --name $containerName `
    --label "llm2api.test.owner=$script:LLM2APITestOwner" `
    --label "llm2api.test.run=$RunID" `
    --publish "127.0.0.1::6379" `
    valkey/valkey:9.1.0-alpine `
    valkey-server --appendonly no --requirepass $Password
  $containerExitCode = $LASTEXITCODE
  $container = [string] ($containerOutput | Select-Object -First 1)
  $container = $container.Trim()
  if ($containerExitCode -ne 0 -or -not $container) {
    throw "Could not start isolated Valkey."
  }

  try {
    Wait-LLM2APIValkeyReady -Container $container -Password $Password
    $port = Get-LLM2APIPublishedPort -Container $container -Target "6379/tcp"
    return [pscustomobject]@{
      Container = $container
      Address   = "127.0.0.1:$port"
    }
  } catch {
    $startFailure = $_
    try {
      Stop-LLM2APITestContainer -Container $container -RunID $RunID
    } catch {
      throw "Valkey startup failed: $($startFailure.Exception.Message) Cleanup also failed: $($_.Exception.Message)"
    }
    throw $startFailure
  }
}

function Stop-LLM2APITestContainer {
  param(
    [string] $Container,
    [Parameter(Mandatory = $true)][string] $RunID
  )

  if (-not $Container) {
    return
  }
  if ($RunID -notmatch '^[a-z][a-z0-9-]{1,63}$') {
    throw "Refusing to use an invalid isolated test-run ID."
  }
  if ($Container -notmatch '^[a-f0-9]{12,64}$') {
    throw "Refusing to inspect an invalid isolated container ID."
  }
  $docker = Get-LLM2APIDockerCommand
  $inspectionProbe = Invoke-LLM2APINativeProbe { & $docker inspect $Container }
  if ($inspectionProbe.ExitCode -ne 0) {
    $existenceProbe = Invoke-LLM2APINativeProbe { & $docker ps --all --quiet --no-trunc --filter "id=$Container" }
    if ($existenceProbe.ExitCode -ne 0) {
      throw "Could not determine whether isolated container $Container still exists."
    }
    if ($existenceProbe.Output.Count -eq 0) {
      return
    }
    throw "Could not inspect isolated container $Container before cleanup."
  }
  $inspection = @($inspectionProbe.Output | ConvertFrom-Json)
  if ($inspection.Count -ne 1) {
    throw "Docker returned an invalid inspection result for isolated container $Container."
  }
  $actualOwner = $inspection[0].Config.Labels.'llm2api.test.owner'
  $actualRunID = $inspection[0].Config.Labels.'llm2api.test.run'
  $actualName = [string] $inspection[0].Name
  $expectedNames = @("/llm2api-test-postgres-$RunID", "/llm2api-test-valkey-$RunID")
  if ($actualOwner -ne $script:LLM2APITestOwner -or $actualRunID -ne $RunID -or $actualName -notin $expectedNames) {
    throw "Refusing to remove container $Container because its isolated-test ownership does not match."
  }
  & $docker rm --force $Container *> $null
  if ($LASTEXITCODE -ne 0) {
    throw "Could not remove isolated container $Container."
  }
}

function Save-LLM2APIEnvironment {
  $snapshot = @{}
  foreach ($item in Get-ChildItem Env: | Where-Object { $_.Name -like "LLM2API_*" }) {
    $snapshot[$item.Name] = $item.Value
  }
  return $snapshot
}

function Clear-LLM2APIEnvironment {
  foreach ($item in @(Get-ChildItem Env: | Where-Object { $_.Name -like "LLM2API_*" })) {
    [Environment]::SetEnvironmentVariable($item.Name, $null, "Process")
  }
}

function Restore-LLM2APIEnvironment {
  param([Parameter(Mandatory = $true)][hashtable] $Snapshot)

  Clear-LLM2APIEnvironment
  foreach ($name in $Snapshot.Keys) {
    [Environment]::SetEnvironmentVariable($name, $Snapshot[$name], "Process")
  }
}
