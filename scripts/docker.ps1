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
