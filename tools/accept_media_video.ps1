# Compatible with Windows PowerShell 5.1 and PowerShell 7. Never automatically retries POST.
[CmdletBinding()]
param(
    [string]$BaseURL = 'http://127.0.0.1:18088',
    [string]$TaskID,
    [string]$IdempotencyKey,
    [string]$Prompt = '一个火柴人在白色背景中行走',
    [string]$OutputFile = 'accepted-video.mp4'
)
$ErrorActionPreference = 'Stop'
$secureKey = Read-Host 'Sub2API API Key（不是供应商密钥）' -AsSecureString
$credential = [System.Net.NetworkCredential]::new('', $secureKey)
$headers = @{ Authorization = 'Bearer ' + $credential.Password }
$BaseURL = $BaseURL.TrimEnd('/')

function Show-Balance {
    $wallet = Invoke-RestMethod -Uri "$BaseURL/v1/media/billing" -Headers $headers
    Write-Host ('可用余额 {0}；冻结余额 {1}；新订单单价 {2}' -f $wallet.balance, $wallet.frozen_balance, $wallet.video_price_per_request)
    return $wallet
}

$before = Show-Balance
if (-not $TaskID) {
    if ($IdempotencyKey) {
        $headers['X-Media-Video-Replay-Only'] = 'true'
        Write-Host '按原幂等键只读恢复；不会创建新订单或再次提交供应商。'
    } else {
        if (-not $before.can_create) { throw '可用余额不足，未提交生成请求。' }
        $confirmation = Read-Host '下面会真实创建一次视频并冻结余额，供应商可能计费。输入 GENERATE 才继续'
        if ($confirmation -cne 'GENERATE') { Write-Host '已退出，未提交。'; return }
    }
    $idem = $IdempotencyKey
    if (-not $idem) { $idem = 'manual-video-' + [guid]::NewGuid().ToString('N') }
    Write-Host "本次幂等键：$idem（请保存；网络异常时不要用新键重建）"
    $headers['Idempotency-Key'] = $idem
    $body = @{ model='seedance2.5'; prompt=$Prompt; duration=30; ratio='16:9'; resolution='720p' } | ConvertTo-Json -Compress
    try {
        $task = Invoke-RestMethod -Method Post -Uri "$BaseURL/v1/media/videos" -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ([Text.Encoding]::UTF8.GetBytes($body))
    } catch {
        Write-Host "提交结果未确认。保留原 Prompt，人工重试时传入 -IdempotencyKey $idem；不要生成新键。"
        throw
    }
    $TaskID = $task.task_id
    Write-Host "本站任务 ID：$TaskID"
    $null = Show-Balance
}

$deadline = [DateTimeOffset]::UtcNow.AddHours(24)
do {
    $task = Invoke-RestMethod -Uri "$BaseURL/v1/media/videos/$TaskID" -Headers $headers
    Write-Host ('状态 {0}；账务 {1}；可下载 {2}；订单价格 {3}' -f $task.status,$task.billing_status,$task.downloadable,$task.price)
    if ($task.status -eq 'failed') { $null=Show-Balance; throw ('视频失败：'+($task.error | ConvertTo-Json -Compress)) }
    if ($task.downloadable -and $task.billing_status -eq 'settled') { break }
    if ([DateTimeOffset]::UtcNow -ge $deadline) { throw '等待超时，请用 -TaskID 查询原任务，不要重新创建。' }
    Start-Sleep -Seconds 15
} while ($true)

# .NET Framework requires AddRange rather than setting the restricted Range header.
$probeRequest = [System.Net.HttpWebRequest]::Create("$BaseURL/v1/media/videos/$TaskID/content")
$probeRequest.Headers['Authorization'] = $headers.Authorization
$probeRequest.AllowAutoRedirect = $false
$probeRequest.Timeout = 30000
$probeRequest.AddRange(0, 0)
$probe = $probeRequest.GetResponse()
try {
    Write-Host ('Range 下载状态：{0}；类型：{1}' -f [int]$probe.StatusCode,$probe.ContentType)
    if ([int]$probe.StatusCode -ne 206) { throw 'Range 未返回 206，验收未通过。' }
    if (-not $probe.ContentType.StartsWith('video/mp4')) { throw '下载内容不是 video/mp4，验收未通过。' }
    if (-not $probe.Headers['Content-Range']) { throw '缺少 Content-Range，验收未通过。' }
    if ($probe.GetResponseStream().ReadByte() -lt 0) { throw '视频内容为空，验收未通过。' }
} finally {
    $probe.Close()
}
Invoke-WebRequest -Uri "$BaseURL/v1/media/videos/$TaskID/content" -Headers $headers -UseBasicParsing -OutFile $OutputFile
$null = Show-Balance
Write-Host "视频保存到 $OutputFile。请人工播放确认内容完整；重复下载应不再扣款。"
