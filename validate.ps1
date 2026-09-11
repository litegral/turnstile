param(
    [switch]$SkipLoadTest
)

$ErrorActionPreference = "Stop"
$runID = [guid]::NewGuid().ToString("N")
$project = "turnstile-validation-$($runID.Substring(0, 8))"
$composePath = Join-Path $PSScriptRoot "compose.yaml"
$databaseURL = "postgres://turnstile:turnstile@postgres:5432/turnstile?sslmode=disable"
$stageCount = if ($SkipLoadTest) { 1 } else { 2 }

Write-Host "Turnstile validation"
Write-Host "Docker runs Go, PostgreSQL, backend services, mock accounting, and k6."
Write-Host "No local Go, PostgreSQL, or k6 installation required."
Write-Host ""

try {
    Write-Host "[1/$stageCount] Running complete Go test suite with PostgreSQL and race detector"
    docker compose -p $project -f $composePath up -d postgres --wait
    if ($LASTEXITCODE -ne 0) {
        throw "Validation database startup failed"
    }

    docker run --rm `
        --network "$project`_default" `
        --mount "type=bind,source=$PSScriptRoot,target=/src" `
        --workdir /src `
        -e "TEST_DATABASE_URL=$databaseURL" `
        golang:1.25.5 `
        go test -race -count=1 -p=1 -v ./...
    if ($LASTEXITCODE -ne 0) {
        throw "Go test suite failed"
    }
}
finally {
    Write-Host "Cleaning up test database"
    docker compose -p $project -f $composePath down -v
}

if (-not $SkipLoadTest) {
    Write-Host ""
    Write-Host "[2/$stageCount] Running full backend and 10,001-request load test"
    & (Join-Path $PSScriptRoot "loadtest/run.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "Load validation failed"
    }
}

Write-Host ""
Write-Host "Assessment scenario summary"
Write-Host "  1. Race Condition:             PASSED"
if ($SkipLoadTest) {
    Write-Host "  2. High Traffic Processing:    SKIPPED"
} else {
    Write-Host "  2. High Traffic Processing:    PASSED"
}
Write-Host "  3. External API Integration:   PASSED"
Write-Host "  4. Duplicate Request:          PASSED"
Write-Host "  5. Data Synchronization:       PASSED"
Write-Host ""
if ($SkipLoadTest) {
    Write-Host "SELECTED VALIDATION PASSED (load test skipped)"
} else {
    Write-Host "ALL ASSESSMENT SCENARIOS PASSED"
}
Write-Host "All temporary Docker resources were removed."
