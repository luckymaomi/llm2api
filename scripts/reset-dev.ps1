[CmdletBinding()]
param(
  [switch] $ConfirmDataLoss
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\docker.ps1"
. "$PSScriptRoot\dev-processes.ps1"

function Get-LLM2APIDockerLabels {
  param(
    [Parameter(Mandatory = $true)][ValidateSet("container", "volume")][string] $ResourceType,
    [Parameter(Mandatory = $true)][string] $Name
  )

  $docker = Get-LLM2APIDockerCommand
  if ($ResourceType -eq "container") {
    $names = @(& $docker container ls --all --format '{{.Names}}')
  } else {
    $names = @(& $docker volume ls --format '{{.Name}}')
  }
  if ($LASTEXITCODE -ne 0) {
    throw "Could not list Docker $ResourceType resources before reset."
  }
  if ($names -notcontains $Name) {
    return $null
  }
  if ($ResourceType -eq "container") {
    $encoded = & $docker container inspect --format '{{json .Config.Labels}}' $Name
  } else {
    $encoded = & $docker volume inspect --format '{{json .Labels}}' $Name
  }
  if ($LASTEXITCODE -ne 0 -or -not $encoded) {
    throw "Could not inspect Docker ${ResourceType} ownership before reset: $Name"
  }
  $labels = ConvertFrom-Json -InputObject ($encoded -join "")
  if ($null -eq $labels) {
    throw "Docker $ResourceType does not expose ownership labels: $Name"
  }
  return $labels
}

function Assert-LLM2APIOwnedContainerIfPresent {
  param(
    [Parameter(Mandatory = $true)][string] $Name,
    [Parameter(Mandatory = $true)][string] $Service
  )

  $labels = Get-LLM2APIDockerLabels -ResourceType container -Name $Name
  if ($null -eq $labels) {
    return
  }
  if ($labels.'com.docker.compose.project' -ne "llm2api" -or
      $labels.'com.docker.compose.service' -ne $Service) {
    throw "Refusing to reset $Name because it is not owned by the expected LLM2API Compose service."
  }
}

function Assert-LLM2APIOwnedVolumeIfPresent {
  param(
    [Parameter(Mandatory = $true)][string] $Name,
    [Parameter(Mandatory = $true)][string] $LogicalName
  )

  $labels = Get-LLM2APIDockerLabels -ResourceType volume -Name $Name
  if ($null -eq $labels) {
    return
  }
  if ($labels.'com.docker.compose.project' -ne "llm2api" -or
      $labels.'com.docker.compose.volume' -ne $LogicalName) {
    throw "Refusing to reset $Name because it is not owned by the expected LLM2API Compose volume."
  }
}

function Assert-LLM2APIResourceAbsent {
  param(
    [Parameter(Mandatory = $true)][ValidateSet("container", "volume")][string] $ResourceType,
    [Parameter(Mandatory = $true)][string] $Name
  )

  if ($null -ne (Get-LLM2APIDockerLabels -ResourceType $ResourceType -Name $Name)) {
    throw "Reset did not remove the expected ${ResourceType}: $Name"
  }
}

if (-not $ConfirmDataLoss) {
  throw "Data reset requires the explicit -ConfirmDataLoss switch."
}

$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
Stop-LLM2APIDevelopmentProcesses -Root $root

Assert-LLM2APIOwnedContainerIfPresent -Name "llm2api-postgres" -Service "postgres"
Assert-LLM2APIOwnedContainerIfPresent -Name "llm2api-valkey" -Service "valkey"
Assert-LLM2APIOwnedVolumeIfPresent -Name "llm2api_postgres" -LogicalName "llm2api_postgres"
Assert-LLM2APIOwnedVolumeIfPresent -Name "llm2api_valkey" -LogicalName "llm2api_valkey"

Push-Location $root
try {
  Write-Host "Removing LLM2API development containers and named data volumes..."
  Invoke-LLM2APIDocker compose down --volumes
} finally {
  Pop-Location
}

Assert-LLM2APIResourceAbsent -ResourceType container -Name "llm2api-postgres"
Assert-LLM2APIResourceAbsent -ResourceType container -Name "llm2api-valkey"
Assert-LLM2APIResourceAbsent -ResourceType volume -Name "llm2api_postgres"
Assert-LLM2APIResourceAbsent -ResourceType volume -Name "llm2api_valkey"

Write-Host "LLM2API local PostgreSQL and Valkey data were reset. Source files and other Docker projects were not changed."
