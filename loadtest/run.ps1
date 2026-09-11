param(
    [ValidateRange(10001, 1000000)]
    [int]$RequestCount = 10001,
    [ValidateRange(1, 2000)]
    [int]$VUs = 200
)

$ErrorActionPreference = "Stop"
$runID = [guid]::NewGuid().ToString("N")
$project = "turnstile-load-$($runID.Substring(0, 8))"
$composePath = Join-Path $PSScriptRoot "../compose.yaml"
$keyPrefix = "load-$runID-"
$resultsDirectory = Join-Path $PSScriptRoot "results"
$countsPath = Join-Path $resultsDirectory "counts.txt"
$summaryPath = Join-Path $resultsDirectory "summary.json"

$env:POSTGRES_DB = "turnstile"
$env:POSTGRES_USER = "turnstile"
$env:POSTGRES_PASSWORD = "turnstile"
$env:API_PORT = "0"
$env:LOG_LEVEL = "warn"

New-Item -ItemType Directory -Force -Path $resultsDirectory | Out-Null
Remove-Item -LiteralPath $countsPath, $summaryPath -ErrorAction SilentlyContinue

try {
    Write-Host "[1/4] Starting full backend stack"
    docker compose -p $project -f $composePath up -d --build --wait
    if ($LASTEXITCODE -ne 0) {
        throw "Docker Compose startup failed"
    }

    Write-Host "[2/4] Creating inventory for $RequestCount bookings"
    $setupSQL = Get-Content -Raw (Join-Path $PSScriptRoot "setup.sql")
    $inventoryID = ($setupSQL | docker compose -p $project -f $composePath exec -T postgres psql `
        -qAt -v ON_ERROR_STOP=1 -v "run_id=$runID" -v "request_count=$RequestCount" `
        -U turnstile -d turnstile).Trim()
    if ($LASTEXITCODE -ne 0 -or $inventoryID -notmatch '^\d+$') {
        throw "Inventory setup failed: $inventoryID"
    }

    Write-Host "[3/4] Sending $RequestCount bookings with $VUs virtual users"
    docker pull grafana/k6:1.3.0
    if ($LASTEXITCODE -ne 0) {
        throw "k6 image pull failed"
    }
    docker run --rm `
        --network "$project`_default" `
        --mount "type=bind,source=$PSScriptRoot,target=/work" `
        -e "BASE_URL=http://api:8080" `
        -e "RUN_ID=$runID" `
        -e "INVENTORY_ID=$inventoryID" `
        -e "REQUEST_COUNT=$RequestCount" `
        -e "VUS=$VUs" `
        grafana/k6:1.3.0 run /work/bookings.js
    $k6Exit = $LASTEXITCODE

    if (-not (Test-Path -LiteralPath $countsPath) -or -not (Test-Path -LiteralPath $summaryPath)) {
        throw "k6 did not produce files in $resultsDirectory"
    }
    $counts = (Get-Content -Raw $countsPath).Trim() -split '\|'
    if ($counts.Count -ne 3) {
        throw "Invalid k6 counts: $($counts -join '|')"
    }
    $httpRequests = [int64]$counts[0]
    $httpSuccesses = [int64]$counts[1]
    $elapsedMilliseconds = [double]$counts[2]

    Write-Host "[4/4] Reconciling HTTP results with PostgreSQL"
    $verifySQL = Get-Content -Raw (Join-Path $PSScriptRoot "verify.sql")
    $verification = ($verifySQL | docker compose -p $project -f $composePath exec -T postgres psql `
        -qAt -F "|" -v ON_ERROR_STOP=1 -v "inventory_id=$inventoryID" `
        -v "key_prefix=$keyPrefix" -v "expected=$RequestCount" -v "http_successes=$httpSuccesses" `
        -U turnstile -d turnstile).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "Database verification failed"
    }

    $fields = $verification -split '\|'
    $passed = $k6Exit -eq 0 `
        -and $httpRequests -eq $RequestCount `
        -and $httpSuccesses -eq $RequestCount `
        -and $elapsedMilliseconds -lt 60000 `
        -and $fields.Count -eq 9 `
        -and $fields[8] -eq "t"

    Write-Host ""
    Write-Host "High-traffic evidence"
    Write-Host "  Requests:              $httpRequests"
    Write-Host "  Created responses:     $httpSuccesses"
    Write-Host "  Elapsed seconds:       $([math]::Round($elapsedMilliseconds / 1000, 3))"
    Write-Host "  Database verification: $verification"
    Write-Host "  Passed:                $passed"

    if (-not $passed) {
        throw "High-traffic validation failed"
    }
}
finally {
    Write-Host "Cleaning up load-test containers and database"
    docker compose -p $project -f $composePath down -v
}
