param(
    [switch]$SkipLoadTest
)

$ErrorActionPreference = "Stop"
$runID = [guid]::NewGuid().ToString("N")
$project = "turnstile-validation-$($runID.Substring(0, 8))"
$databaseURL = "postgres://turnstile:turnstile@postgres:5432/turnstile?sslmode=disable"

$env:POSTGRES_DB = "turnstile"
$env:POSTGRES_USER = "turnstile"
$env:POSTGRES_PASSWORD = "turnstile"

try {
    docker compose -p $project up -d postgres --wait
    if ($LASTEXITCODE -ne 0) {
        throw "validation database startup failed"
    }

    docker run --rm `
        --network "$project`_default" `
        --mount "type=bind,source=$PSScriptRoot,target=/src" `
        --workdir /src `
        -e "TEST_DATABASE_URL=$databaseURL" `
        golang:1.25.5 `
        go test -race -count=1 -p=1 -v `
        ./internal/booking ./internal/accounting ./internal/payment ./internal/availability
    if ($LASTEXITCODE -ne 0) {
        throw "acceptance validation failed"
    }
}
finally {
    docker compose -p $project down -v
}

if (-not $SkipLoadTest) {
    & (Join-Path $PSScriptRoot "loadtest\run.ps1")
    if ($LASTEXITCODE -ne 0) {
        throw "load validation failed"
    }
}
