param(
    [switch]$SkipLoadTest
)

$ErrorActionPreference = "Stop"
$runID = [guid]::NewGuid().ToString("N")
$project = "turnstile-validation-$($runID.Substring(0, 8))"
$composePath = Join-Path $PSScriptRoot "compose.yaml"
$databaseURL = "postgres://turnstile:turnstile@postgres:5432/turnstile?sslmode=disable"
$testLogPath = Join-Path ([System.IO.Path]::GetTempPath()) "turnstile-tests-$runID.log"
$loadEvidencePath = Join-Path $PSScriptRoot "loadtest/results/evidence.txt"
$stageCount = if ($SkipLoadTest) { 1 } else { 2 }

Write-Host "Turnstile validation"
Write-Host "Docker runs Go, PostgreSQL, backend services, mock accounting, and k6."
Write-Host "No local Go, PostgreSQL, or k6 installation required."
Write-Host ""

try {
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
            go test -race -count=1 -p=1 -v ./... 2>&1 | Tee-Object -FilePath $testLogPath
        if ($LASTEXITCODE -ne 0) {
            throw "Go test suite failed"
        }
    }
    finally {
        Write-Host "Cleaning up test database"
        docker compose -p $project -f $composePath down -v
    }

    $testOutput = Get-Content -LiteralPath $testLogPath
    $raceEvidence = $testOutput | Where-Object { $_ -match 'race evidence:' } | Select-Object -First 1
    $accountingEvidence = $testOutput | Where-Object { $_ -match 'accounting evidence:' } | Select-Object -First 1
    $duplicateEvidence = $testOutput | Where-Object { $_ -match 'duplicate webhook evidence:' } | Select-Object -First 1
    $availabilityEvidence = $testOutput | Where-Object { $_ -match 'availability evidence:' } | Select-Object -First 1
    if (-not $raceEvidence -or -not $accountingEvidence -or -not $duplicateEvidence -or -not $availabilityEvidence) {
        throw "Go tests passed but one or more assessment evidence markers are missing"
    }

    if (-not $SkipLoadTest) {
        Write-Host ""
        Write-Host "[2/$stageCount] Running full backend and 10,001-request load test"
        & (Join-Path $PSScriptRoot "loadtest/run.ps1")
        if ($LASTEXITCODE -ne 0) {
            throw "Load validation failed"
        }
        if (-not (Test-Path -LiteralPath $loadEvidencePath)) {
            throw "High-traffic evidence file is missing"
        }
        $highTrafficEvidence = (Get-Content -Raw -LiteralPath $loadEvidencePath).Trim()
    }

    Write-Host ""
    Write-Host "Assessment scenario summary"
    Write-Host "  1. Race Condition:             PASSED"
    Write-Host "     $($raceEvidence -replace '^.*race evidence: ', '')"
    if ($SkipLoadTest) {
        Write-Host "  2. High Traffic Processing:    SKIPPED"
    } else {
        Write-Host "  2. High Traffic Processing:    PASSED"
        Write-Host "     $highTrafficEvidence"
    }
    Write-Host "  3. External API Integration:   PASSED"
    Write-Host "     $($accountingEvidence -replace '^.*accounting evidence: ', '')"
    Write-Host "  4. Duplicate Request:          PASSED"
    Write-Host "     $($duplicateEvidence -replace '^.*duplicate webhook evidence: ', '')"
    Write-Host "  5. Data Synchronization:       PASSED"
    Write-Host "     $($availabilityEvidence -replace '^.*availability evidence: ', '')"
    Write-Host ""
    if ($SkipLoadTest) {
        Write-Host "SELECTED VALIDATION PASSED (load test skipped)"
    } else {
        Write-Host "ALL ASSESSMENT SCENARIOS PASSED"
    }
    Write-Host "All temporary Docker resources were removed."
}
finally {
    Remove-Item -LiteralPath $testLogPath -ErrorAction SilentlyContinue
}
