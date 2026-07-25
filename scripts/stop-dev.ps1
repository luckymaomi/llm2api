$ErrorActionPreference = "Stop"

. "$PSScriptRoot\docker.ps1"
. "$PSScriptRoot\dev-processes.ps1"

$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
Stop-LLM2APIDevelopmentProcesses -Root $root

Push-Location $root
try {
  Invoke-LLM2APIDocker compose down
  Write-Host "LLM2API development processes and infrastructure stopped. Named volumes were preserved."
} finally {
  Pop-Location
}
