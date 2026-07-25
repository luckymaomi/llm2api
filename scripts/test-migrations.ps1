$ErrorActionPreference = "Stop"

. "$PSScriptRoot\isolated-services.ps1"

Push-Location (Join-Path $PSScriptRoot "..")
$runID = New-LLM2APITestRunID -Purpose "migrations"
$postgres = $null
$environmentSnapshot = Save-LLM2APIEnvironment
$testFailure = $null
try {
  Clear-LLM2APIEnvironment
  $postgres = Start-LLM2APITestPostgres -RunID $runID -DatabaseName "llm2api_migrations" -Password "migration-postgres-fixture"

  $env:LLM2API_PROFILE = "test"
  $env:LLM2API_DATABASE_URL = $postgres.DatabaseURL
  $env:LLM2API_MASTER_KEYS = "1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
  $env:LLM2API_SESSION_PEPPER = "llm2api-test-session-pepper-000000"
  $env:LLM2API_API_KEY_PEPPER = "llm2api-test-api-key-pepper-000000"

  & go run .\cmd\dbtool --action up
  if ($LASTEXITCODE -ne 0) {
    throw "Migration up failed."
  }

  $docker = Get-LLM2APIDockerCommand
  $tableCount = & $docker exec $postgres.Container psql -v ON_ERROR_STOP=1 -U llm2api -d llm2api_migrations -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'"
  if ($LASTEXITCODE -ne 0 -or [int]$tableCount -lt 20) {
    throw "Migration schema is incomplete; found $tableCount public tables."
  }

  & go run .\cmd\dbtool --action rebuild --confirm-development-data-loss
  if ($LASTEXITCODE -ne 0) {
    throw "Migration rebuild failed."
  }

  $rebuiltTableCount = & $docker exec $postgres.Container psql -v ON_ERROR_STOP=1 -U llm2api -d llm2api_migrations -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'"
  if ($LASTEXITCODE -ne 0 -or [int]$rebuiltTableCount -lt 20) {
    throw "Migration rebuild did not restore the schema; found $rebuiltTableCount public tables."
  }

} catch {
  $testFailure = $_
} finally {
  $cleanupFailures = @()
  try {
    Restore-LLM2APIEnvironment -Snapshot $environmentSnapshot
  } catch {
    $cleanupFailures += "environment restore: $($_.Exception.Message)"
  }
  if ($null -ne $postgres) {
    try {
      Stop-LLM2APITestContainer -Container $postgres.Container -RunID $runID
    } catch {
      $cleanupFailures += "PostgreSQL cleanup: $($_.Exception.Message)"
    }
  }
  try {
    Pop-Location
  } catch {
    $cleanupFailures += "location restore: $($_.Exception.Message)"
  }
  if ($null -ne $testFailure) {
    if ($cleanupFailures.Count -gt 0) {
      throw "Migration round-trip failed: $($testFailure.Exception.Message) Cleanup also failed: $($cleanupFailures -join '; ')"
    }
    throw $testFailure
  }
  if ($cleanupFailures.Count -gt 0) {
    throw "Migration round-trip cleanup failed: $($cleanupFailures -join '; ')"
  }
}

Write-Host "Migration round-trip passed in an isolated PostgreSQL container."
