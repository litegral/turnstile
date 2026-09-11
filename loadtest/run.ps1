param(
    [ValidateRange(10001, 1000000)]
    [int]$RequestCount = 10001,
    [ValidateRange(1, 2000)]
    [int]$VUs = 200
)

$ErrorActionPreference = "Stop"
$runID = [guid]::NewGuid().ToString("N")
$project = "turnstile-load-$($runID.Substring(0, 8))"
$keyPrefix = "load-$runID-"
$summaryPath = Join-Path $PSScriptRoot "results\summary.json"

$env:POSTGRES_DB = "turnstile"
$env:POSTGRES_USER = "turnstile"
$env:POSTGRES_PASSWORD = "turnstile"
$env:API_PORT = "0"
$env:LOG_LEVEL = "warn"

New-Item -ItemType Directory -Force -Path (Split-Path $summaryPath) | Out-Null
Remove-Item -LiteralPath $summaryPath -ErrorAction SilentlyContinue

docker compose -p $project up -d --build --wait
if ($LASTEXITCODE -ne 0) {
    throw "docker compose startup failed"
}

$setupSQL = Get-Content -Raw (Join-Path $PSScriptRoot "setup.sql")
$inventoryID = ($setupSQL | docker compose -p $project exec -T postgres psql `
    -qAt -v ON_ERROR_STOP=1 -v "run_id=$runID" -v "request_count=$RequestCount" `
    -U turnstile -d turnstile).Trim()
if ($LASTEXITCODE -ne 0 -or $inventoryID -notmatch '^\d+$') {
    throw "inventory setup failed: $inventoryID"
}

docker pull grafana/k6:1.3.0
if ($LASTEXITCODE -ne 0) {
    throw "k6 image pull failed"
}

$timer = [System.Diagnostics.Stopwatch]::StartNew()
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
$timer.Stop()

if (-not (Test-Path -LiteralPath $summaryPath)) {
    throw "k6 did not produce $summaryPath"
}

$summary = Get-Content -Raw $summaryPath | ConvertFrom-Json
$httpRequests = [int64]$summary.metrics.http_reqs.values.count
$httpSuccesses = [int64]$summary.metrics.booking_created.values.count
$verifySQL = Get-Content -Raw (Join-Path $PSScriptRoot "verify.sql")
$verification = ($verifySQL | docker compose -p $project exec -T postgres psql `
    -qAt -F "|" -v ON_ERROR_STOP=1 -v "inventory_id=$inventoryID" `
    -v "key_prefix=$keyPrefix" -v "expected=$RequestCount" -v "http_successes=$httpSuccesses" `
    -U turnstile -d turnstile).Trim()
if ($LASTEXITCODE -ne 0) {
    throw "database verification failed"
}

$fields = $verification -split '\|'
$passed = $k6Exit -eq 0 `
    -and $httpRequests -eq $RequestCount `
    -and $httpSuccesses -eq $RequestCount `
    -and $timer.Elapsed.TotalSeconds -lt 60 `
    -and $fields.Count -eq 9 `
    -and $fields[8] -eq "t"

[pscustomobject]@{
    RunID = $runID
    InventoryID = $inventoryID
    Requests = $httpRequests
    CreatedResponses = $httpSuccesses
    ElapsedSeconds = [math]::Round($timer.Elapsed.TotalSeconds, 3)
    DatabaseVerification = $verification
    Passed = $passed
} | Format-List

docker compose -p $project down -v
if ($LASTEXITCODE -ne 0) {
    throw "load test cleanup failed"
}

if (-not $passed) {
    exit 1
}
