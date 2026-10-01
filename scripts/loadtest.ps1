param(
    [int]$Requests = 200,
    [int]$UniquePaths = 5,
    [string]$RouterUrl = "http://localhost",
    [string]$EdgeMetricsUrl = "http://localhost:8081/metrics"
)

[System.Net.ServicePointManager]::DefaultConnectionLimit = 100

function Get-Metric([string]$name) {
    try {
        $req = [System.Net.WebRequest]::Create($EdgeMetricsUrl)
        $req.Timeout = 5000
        $resp = $req.GetResponse()
        $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
        $content = $reader.ReadToEnd()
        $reader.Close(); $resp.Close()
        $line = ($content -split "`n") | Where-Object { $_ -match "^$name\{" } | Select-Object -First 1
        if ($line -match "$name\{[^}]*\}\s+(\d+)") { return [int]$Matches[1] }
    } catch {}
    return 0
}

function Invoke-Get([string]$url) {
    $req = [System.Net.WebRequest]::Create($url)
    $req.Method = "GET"
    $req.Timeout = 10000
    $resp = $req.GetResponse()
    $cache = $resp.Headers["X-Cache"]
    $resp.Close()
    return $cache
}

$hitsBefore = Get-Metric "cdn_cache_hits_total"
$missBefore = Get-Metric "cdn_cache_misses_total"

$sw = [System.Diagnostics.Stopwatch]::StartNew()
$hit = 0; $miss = 0; $err = 0
$latencies = [System.Collections.Generic.List[double]]::new()

for ($i = 1; $i -le $Requests; $i++) {
    $path = "/api/v1/load-$($i % $UniquePaths)"
    $reqSw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $cache = Invoke-Get "$RouterUrl$path"
        $reqSw.Stop()
        $latencies.Add($reqSw.Elapsed.TotalMilliseconds)
        if ($cache -eq 'HIT') { $hit++ } else { $miss++ }
    } catch {
        $reqSw.Stop()
        $err++
    }
}
$sw.Stop()

$hitsAfter = Get-Metric "cdn_cache_hits_total"
$missAfter = Get-Metric "cdn_cache_misses_total"

$avg = if ($latencies.Count -gt 0) { [math]::Round(($latencies | Measure-Object -Average).Average, 1) } else { 0 }
$p95 = if ($latencies.Count -gt 0) {
    $sorted = $latencies | Sort-Object
    [math]::Round($sorted[[int][math]::Floor($sorted.Count * 0.95) - 1], 1)
} else { 0 }

Write-Output "=========================================="
Write-Output " Load Test Results"
Write-Output "=========================================="
Write-Output " Requests      : $Requests"
Write-Output " Unique paths  : $UniquePaths"
Write-Output " Errors        : $err"
Write-Output " HIT / MISS    : $hit / $miss"
Write-Output " Client ratio  : $([math]::Round($hit / [math]::Max(1, $hit + $miss) * 100, 1))%"
if (($hitsAfter - $hitsBefore) + ($missAfter - $missBefore) -gt 0) {
    $deltaRatio = ($hitsAfter - $hitsBefore) / (($hitsAfter - $hitsBefore) + ($missAfter - $missBefore)) * 100
    Write-Output " Metrics ratio : $([math]::Round($deltaRatio, 1))%"
}
Write-Output " Total time    : $([math]::Round($sw.Elapsed.TotalSeconds, 2))s"
Write-Output " Throughput    : $([math]::Round($Requests / $sw.Elapsed.TotalSeconds, 1)) req/s"
Write-Output " Avg latency   : $avg ms"
Write-Output " P95 latency   : $p95 ms"
Write-Output "=========================================="