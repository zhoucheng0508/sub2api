[CmdletBinding()]
param(
    [string]$BaseUrl = "https://image.vote520.com",
    [string]$ApiKey = $env:IMAGE_GATEWAY_API_KEY,
    [switch]$AllowHttpLocalhost
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Net.Http

function Assert-Condition {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Get-HeaderValues {
    param([System.Net.Http.HttpResponseMessage]$Response, [string]$Name)
    $values = [System.Collections.Generic.IEnumerable[string]]$null
    if ($Response.Headers.TryGetValues($Name, [ref]$values)) {
        return @($values)
    }
    if ($Response.Content.Headers.TryGetValues($Name, [ref]$values)) {
        return @($values)
    }
    return @()
}

function Invoke-GatewayProbe {
    param(
        [System.Net.Http.HttpClient]$Client,
        [System.Uri]$Origin,
        [string]$Method,
        [string]$Path,
        [string]$RequestOrigin = "",
        [bool]$Authenticated = $false
    )

    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), [System.Uri]::new($Origin, $Path))
    try {
        if ($RequestOrigin) {
            [void]$request.Headers.TryAddWithoutValidation("Origin", $RequestOrigin)
            [void]$request.Headers.TryAddWithoutValidation("Access-Control-Request-Method", "POST")
            [void]$request.Headers.TryAddWithoutValidation("Access-Control-Request-Headers", "authorization,content-type,x-request-id")
        }
        if ($Authenticated) {
            $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $ApiKey)
        }
        return $Client.SendAsync($request).GetAwaiter().GetResult()
    }
    finally {
        $request.Dispose()
    }
}

function Assert-NoStore {
    param([System.Net.Http.HttpResponseMessage]$Response, [string]$Context)
    $cacheControl = (Get-HeaderValues $Response "Cache-Control") -join ","
    Assert-Condition ($cacheControl -match "(^|,)\s*no-store\s*(,|$)") "$Context 缺少 Cache-Control: no-store"
}

$base = [System.Uri]$BaseUrl
$isLocalHttp = $AllowHttpLocalhost -and $base.Scheme -eq "http" -and $base.Host -in @("localhost", "127.0.0.1", "::1")
Assert-Condition ($base.IsAbsoluteUri) "BaseUrl must be an absolute URL"
Assert-Condition ($base.Scheme -eq "https" -or $isLocalHttp) "BaseUrl must use HTTPS unless -AllowHttpLocalhost is set for a local test"
Assert-Condition ($base.AbsolutePath -eq "/" -and -not $base.Query -and -not $base.Fragment) "BaseUrl must contain only an origin"

$handler = [System.Net.Http.HttpClientHandler]::new()
$handler.AllowAutoRedirect = $false
$client = [System.Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(30)

try {
    $allowedOrigin = "https://canvas.vote520.com"
    $deniedOrigin = "https://not-allowed.example"

    $allowed = Invoke-GatewayProbe $client $base "OPTIONS" "/v1/images/generations/async" $allowedOrigin
    try {
        Assert-Condition ([int]$allowed.StatusCode -eq 204) "Allowed-origin preflight returned $([int]$allowed.StatusCode), expected 204"
        Assert-Condition (((Get-HeaderValues $allowed "Access-Control-Allow-Origin") -join ",") -eq $allowedOrigin) "Allowed origin did not receive the exact Access-Control-Allow-Origin value"
        Assert-Condition (((Get-HeaderValues $allowed "Access-Control-Allow-Methods") -join ",") -match "(^|,)\s*POST\s*(,|$)") "Preflight response does not allow POST"
        Assert-Condition (((Get-HeaderValues $allowed "Vary") -join ",") -match "(^|,)\s*Origin\s*(,|$)") "Preflight response is missing Vary: Origin"
        Assert-NoStore $allowed "Allowed-origin preflight response"
    }
    finally {
        $allowed.Dispose()
    }

    $denied = Invoke-GatewayProbe $client $base "OPTIONS" "/v1/images/generations/async" $deniedOrigin
    try {
        Assert-Condition ([int]$denied.StatusCode -eq 204) "Denied-origin preflight returned $([int]$denied.StatusCode), expected 204"
        Assert-Condition ((Get-HeaderValues $denied "Access-Control-Allow-Origin").Count -eq 0) "Denied origin received Access-Control-Allow-Origin"
        Assert-NoStore $denied "Denied-origin preflight response"
    }
    finally {
        $denied.Dispose()
    }

    foreach ($path in @("/v1/chat/completions", "/v1/responses", "/api/v1/admin/settings")) {
        $blocked = Invoke-GatewayProbe $client $base "GET" $path
        try {
            Assert-Condition ([int]$blocked.StatusCode -eq 404) "$path returned $([int]$blocked.StatusCode), expected 404"
            Assert-NoStore $blocked $path
        }
        finally {
            $blocked.Dispose()
        }
    }

    $anonymousModels = Invoke-GatewayProbe $client $base "GET" "/v1/models"
    try {
        Assert-Condition ([int]$anonymousModels.StatusCode -in @(401, 403)) "Anonymous /v1/models returned $([int]$anonymousModels.StatusCode), expected 401 or 403"
        Assert-NoStore $anonymousModels "Anonymous /v1/models"
    }
    finally {
        $anonymousModels.Dispose()
    }

    if ([string]::IsNullOrWhiteSpace($ApiKey)) {
        Write-Host "Image gateway preflight passed for routes, CORS, cache policy, and anonymous authentication rejection. Authenticated model checks were skipped."
        exit 0
    }

    $models = Invoke-GatewayProbe $client $base "GET" "/v1/models" "" $true
    try {
        Assert-Condition ([int]$models.StatusCode -eq 200) "/v1/models returned $([int]$models.StatusCode), expected 200"
        Assert-NoStore $models "/v1/models"
        $payload = $models.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
        $modelIds = @($payload.data | ForEach-Object { [string]$_.id })
        Assert-Condition ($modelIds.Count -eq 1 -and $modelIds[0] -eq "gpt-image-2") "Image key must expose exactly gpt-image-2; received: $($modelIds -join ', ')"
    }
    finally {
        $models.Dispose()
    }

    $billing = Invoke-GatewayProbe $client $base "GET" "/v1/sub2api/billing" "" $true
    try {
        Assert-Condition ([int]$billing.StatusCode -eq 200) "/v1/sub2api/billing returned $([int]$billing.StatusCode), expected 200"
        Assert-NoStore $billing "/v1/sub2api/billing"
    }
    finally {
        $billing.Dispose()
    }

    Write-Host "Vote image gateway preflight passed: routes, CORS, cache policy, authentication, and model isolation are valid."
}
finally {
    $client.Dispose()
    $handler.Dispose()
}
