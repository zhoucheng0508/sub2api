# Windows 10/11: PowerShell 5.1 and the Windows SQLite runtime; no elevation.
$ErrorActionPreference = 'Stop'
$utf8 = [Text.UTF8Encoding]::new($false)
$setup = $utf8.GetString([Convert]::FromBase64String($setupPayload)) | ConvertFrom-Json
$db = [IntPtr]::Zero
$transaction = $false
$committed = $false
$backups = @()
$ccExecutable = $null
$setupMutex = $null
$lockHeld = $false
$journalPath = $null
$operationId = [Guid]::NewGuid().ToString('N')

function Get-SetupHash($path) {
    if (!(Test-Path -LiteralPath $path -PathType Leaf)) { return 'absent' }
    $stream = [IO.File]::OpenRead($path)
    $hash = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($hash.ComputeHash($stream))).Replace('-', '') }
    finally { $stream.Dispose(); $hash.Dispose() }
}

function Restore-SetupFiles($files) {
    foreach ($file in $files) {
        $hash = Get-SetupHash $file.target
        if ($hash -ne $file.before -and $hash -ne $file.after) {
            throw ('Configuration changed after interrupted setup; recovery stopped: ' + $file.target)
        }
    }
    foreach ($file in $files) {
        if ((Get-SetupHash $file.target) -eq $file.before) { continue }
        if ($file.existed) { Copy-Item -LiteralPath $file.saved -Destination $file.target -Force }
        elseif (Test-Path -LiteralPath $file.target) { Remove-Item -LiteralPath $file.target -Force }
    }
}

function Write-SetupFile($path, $content) {
    $temp = $path + '.sub2api-' + [Guid]::NewGuid().ToString('N') + '.tmp'
    try {
        [IO.File]::WriteAllText($temp, $content, $utf8)
        Move-Item -LiteralPath $temp -Destination $path -Force
    } finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force }
    }
}

function Quote-Toml([string]$value) {
    # JSON basic string escapes used here are also TOML basic string escapes.
    return ConvertTo-Json -InputObject $value -Compress
}

function Resolve-SetupDirectory([string]$value) {
    if ($value -eq '~') { $value = $HOME }
    elseif ($value.StartsWith('~/') -or $value.StartsWith('~\')) { $value = Join-Path $HOME $value.Substring(2) }
    $value = [Environment]::ExpandEnvironmentVariables($value)
    if (![IO.Path]::IsPathRooted($value)) { throw 'Configuration directories must be absolute paths.' }
    return [IO.Path]::GetFullPath($value)
}

function Read-CodexToml($content, $catalogContent = $null) {
    $command = Get-Command codex.exe -ErrorAction SilentlyContinue
    $executable = if ($command) { $command.Source } else {
        Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'OpenAI\Codex\bin') -Filter codex.exe -Recurse -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty FullName
    }
    if (!$executable) { throw 'Install Codex before setup. Its TOML parser is required to validate your configuration.' }
    $validationDir = Join-Path $backupDir ('v-' + [Guid]::NewGuid().ToString('N').Substring(0,8))
    New-Item -ItemType Directory -Path $validationDir | Out-Null
    $validationContent = $content
    if ($null -ne $catalogContent) {
        $validationCatalog = Join-Path $validationDir 'models.json'
        Write-SetupFile $validationCatalog $catalogContent
        $catalogLine = 'model_catalog_json = ' + (Quote-Toml $validationCatalog.Replace('\','/'))
        $validationContent = [regex]::Replace($validationContent, '(?m)^\s*model_catalog_json\s*=.*$', $catalogLine)
    }
    Write-SetupFile (Join-Path $validationDir 'config.toml') $validationContent
    $info = [Diagnostics.ProcessStartInfo]::new()
    $info.FileName = $executable
    $info.Arguments = 'app-server'
    $info.WorkingDirectory = $validationDir
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardInput = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.EnvironmentVariables['CODEX_HOME'] = $validationDir
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $info
    try {
        $process.Start() | Out-Null
        $errorTask = $process.StandardError.ReadToEndAsync()
        $process.StandardInput.WriteLine('{"id":1,"method":"initialize","params":{"clientInfo":{"name":"sub2api_config_validation","version":"1.0"}}}')
        $process.StandardInput.WriteLine('{"method":"initialized"}')
        $process.StandardInput.WriteLine('{"id":2,"method":"config/read","params":{"includeLayers":false}}')
        $process.StandardInput.WriteLine('{"id":3,"method":"model/list","params":{"includeHidden":true}}')
        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        $parsedConfig = $null
        $catalogValidated = $false
        while ([DateTime]::UtcNow -lt $deadline) {
            $lineTask = $process.StandardOutput.ReadLineAsync()
            $remaining = [Math]::Max(1, [int]($deadline - [DateTime]::UtcNow).TotalMilliseconds)
            if (!$lineTask.Wait($remaining)) { throw 'Codex configuration validation timed out.' }
            if (!$lineTask.Result) { throw 'Codex rejected the TOML configuration. No files were installed.' }
            $message = $lineTask.Result | ConvertFrom-Json
            if ($message.error) {
                $reason = ([string]$message.error.message).Replace([string]$setup.apiKey, '<redacted>')
                throw ('Codex rejected the TOML configuration. No files were installed. ' + $reason)
            }
            if ($message.id -eq 2) { $parsedConfig = $message.result.config }
            if ($message.id -eq 3) {
                if ($null -ne $catalogContent -and @($message.result.data).Count -eq 0) { throw 'Codex rejected the model catalog. No files were installed.' }
                if ($null -ne $catalogContent -and @($message.result.data).Count -ne @($catalog.models).Count) { throw 'Codex did not load every model in the catalog. No files were installed.' }
                $catalogValidated = $true
            }
            if ($null -ne $parsedConfig -and $catalogValidated) { return $parsedConfig }
        }
        throw 'Codex configuration validation timed out.'
    } finally {
        if ($process.Id -and !$process.HasExited) { $process.Kill(); $process.WaitForExit() }
        $process.Dispose()
    }
}

try {
    Write-Host 'Preparing the dedicated CN OAI configuration...'
    $ccDir = if ($env:SUB2API_CC_SWITCH_DIR) { Resolve-SetupDirectory $env:SUB2API_CC_SWITCH_DIR } else { Join-Path $HOME '.cc-switch' }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $lockId = ([BitConverter]::ToString($sha.ComputeHash($utf8.GetBytes($ccDir.ToLowerInvariant())))).Replace('-', '') } finally { $sha.Dispose() }
    $setupMutex = [Threading.Mutex]::new($false, ('Local\Sub2APICNOAI-' + $lockId))
    try { $lockHeld = $setupMutex.WaitOne(0) } catch [Threading.AbandonedMutexException] { $lockHeld = $true }
    if (!$lockHeld) { throw 'Another CN OAI setup is running. Wait for it to finish.' }
    if (@(Get-Process -Name 'cc-switch' -ErrorAction SilentlyContinue).Count) {
        throw 'Save your edits and exit CC Switch from its tray menu, then run this file again. Setup will not terminate the app.'
    }
    $dbPath = Join-Path $ccDir 'cc-switch.db'
    $settingsPath = Join-Path $ccDir 'settings.json'
    if (!(Test-Path -LiteralPath $dbPath -PathType Leaf) -or !(Test-Path -LiteralPath $settingsPath -PathType Leaf)) {
        throw 'Install and open CC Switch once before running this file.'
    }
    $settings = Get-Content -LiteralPath $settingsPath -Raw -Encoding UTF8 | ConvertFrom-Json
    $targetDir = if ($settings.codexConfigDir) { Resolve-SetupDirectory $settings.codexConfigDir } elseif ($env:CODEX_HOME) { Resolve-SetupDirectory $env:CODEX_HOME } else { Join-Path $HOME '.codex' }
    if ($settings.codexConfigDir -and $env:CODEX_HOME -and $targetDir -ne (Resolve-SetupDirectory $env:CODEX_HOME)) {
        throw 'CC Switch and CODEX_HOME point to different directories. Align them before setup.'
    }

    Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class CNOAISqlite {
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_open_v2(byte[] name, out IntPtr db, int flags, IntPtr vfs);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] public static extern int sqlite3_close(IntPtr db);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_busy_timeout(IntPtr db, int ms);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_prepare_v2(IntPtr db, byte[] sql, int len, out IntPtr stmt, IntPtr tail);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_bind_text(IntPtr stmt, int n, byte[] text, int len, IntPtr destructor);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_step(IntPtr stmt);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_finalize(IntPtr stmt);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern IntPtr sqlite3_column_text(IntPtr stmt, int col);
    [DllImport("winsqlite3", CallingConvention=CallingConvention.Cdecl)] static extern int sqlite3_column_bytes(IntPtr stmt, int col);
    static byte[] Bytes(string text) { return Encoding.UTF8.GetBytes(text + "\0"); }
    public static IntPtr Open(string path) {
        IntPtr db; int code = sqlite3_open_v2(Bytes(path), out db, 2, IntPtr.Zero);
        if(code != 0) { if(db != IntPtr.Zero) sqlite3_close(db); throw new Exception("Cannot open CC Switch database: " + code); }
        sqlite3_busy_timeout(db, 5000); return db;
    }
    public static string Run(IntPtr db, string sql, string[] args) {
        IntPtr stmt; int code = sqlite3_prepare_v2(db, Bytes(sql), -1, out stmt, IntPtr.Zero);
        if(code != 0) throw new Exception("Unsupported CC Switch database schema: " + code);
        try {
            for(int i=0;i<args.Length;i++) {
                byte[] bytes = Bytes(args[i]);
                if(sqlite3_bind_text(stmt, i+1, bytes, bytes.Length-1, new IntPtr(-1)) != 0) throw new Exception("Cannot bind database value.");
            }
            code = sqlite3_step(stmt);
            if(code == 101) return null;
            if(code != 100) throw new Exception("CC Switch database operation failed: " + code);
            IntPtr p = sqlite3_column_text(stmt,0); if(p == IntPtr.Zero) return null;
            byte[] value = new byte[sqlite3_column_bytes(stmt,0)]; Marshal.Copy(p,value,0,value.Length); return Encoding.UTF8.GetString(value);
        } finally { sqlite3_finalize(stmt); }
    }
}
'@
    $db = [CNOAISqlite]::Open($dbPath)
    $journalPath = Join-Path $ccDir 'sub2api-cn-oai-pending.json'
    [CNOAISqlite]::Run($db, 'CREATE TABLE IF NOT EXISTS sub2api_cn_oai_operations (id TEXT PRIMARY KEY)', @()) | Out-Null
    if (Test-Path -LiteralPath $journalPath) {
        $pending = Get-Content -LiteralPath $journalPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $installed = [CNOAISqlite]::Run($db, 'SELECT id FROM sub2api_cn_oai_operations WHERE id = ?', @([string]$pending.id))
        if (!$installed) { Restore-SetupFiles $pending.files }
        Remove-Item -LiteralPath $journalPath -Force
    }
    # Schema/proxy checks must pass before stopping the app or writing files.
    [CNOAISqlite]::Run($db, 'SELECT id, app_type, name, settings_config, meta, is_current FROM providers LIMIT 1', @()) | Out-Null
    $proxy = [CNOAISqlite]::Run($db, "SELECT count(*) FROM proxy_config WHERE enabled <> 0 OR live_takeover_active <> 0", @())
    if ($proxy -ne '0') { throw 'Turn off CC Switch proxy takeover before setup; active proxy connections will not be interrupted.' }

    # Interrupted local work is repaired before this run depends on the network.
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $response = Invoke-WebRequest -Uri $setup.manifestUrl -Headers @{ Authorization = 'Bearer ' + $setup.apiKey; Accept = 'application/json' } -UseBasicParsing -TimeoutSec 45 -MaximumRedirection 0
    $catalog = $response.Content | ConvertFrom-Json
    if ($catalog.models -isnot [Array] -or $catalog.models.Count -eq 0) { throw 'The group returned an empty model catalog.' }
    $ids = @{}
    $allowedEfforts = @('none','minimal','low','medium','high','xhigh','max','auto')
    foreach ($entry in $catalog.models) {
        if ($entry.slug -isnot [string] -or [string]::IsNullOrWhiteSpace($entry.slug) -or $entry.slug -match '[\x00-\x1f]' -or $ids.ContainsKey($entry.slug) -or
            $entry.supported_reasoning_levels -isnot [Array] -or !$entry.supported_reasoning_levels.Count -or
            $entry.input_modalities -isnot [Array] -or !$entry.input_modalities.Count -or
            $entry.default_reasoning_level -isnot [string] -or $allowedEfforts -cnotcontains $entry.default_reasoning_level) { throw 'The model catalog is invalid.' }
        $effortsSeen = @{}; $modalitiesSeen = @{}
        foreach ($level in $entry.supported_reasoning_levels) {
            if ($level.effort -isnot [string] -or $allowedEfforts -cnotcontains $level.effort -or $effortsSeen.ContainsKey($level.effort)) { throw 'The model catalog has an invalid or duplicate reasoning effort.' }
            $effortsSeen[$level.effort] = $true
        }
        foreach ($modality in $entry.input_modalities) {
            if ($modality -isnot [string] -or @('text','image') -cnotcontains $modality -or $modalitiesSeen.ContainsKey($modality)) { throw 'The model catalog has an invalid or duplicate input modality.' }
            $modalitiesSeen[$modality] = $true
        }
        $ids[[string]$entry.slug] = $true
        if (@($entry.supported_reasoning_levels.effort) -cnotcontains $entry.default_reasoning_level) { throw 'A model has an invalid default reasoning effort.' }
    }
    $model = if ($setup.model) { @($catalog.models | Where-Object { $_.slug -ceq $setup.model }) | Select-Object -First 1 } else { $catalog.models | Where-Object { $_.supported_reasoning_levels.Count -gt 1 } | Select-Object -First 1 }
    if (!$setup.model -and !$model) { $model = $catalog.models | Select-Object -First 1 }
    if (!$model) { throw 'The selected model is not available in this group.' }

    if (!$ccExecutable) {
        foreach ($candidate in @((Join-Path $env:LOCALAPPDATA 'Programs\CC Switch\cc-switch.exe'), (Join-Path $env:ProgramFiles 'CC Switch\cc-switch.exe'))) {
            if (Test-Path -LiteralPath $candidate -PathType Leaf) { $ccExecutable = $candidate; break }
        }
    }
    # Re-read after shutdown: CC Switch may save its current selection on exit.
    $settings = Get-Content -LiteralPath $settingsPath -Raw -Encoding UTF8 | ConvertFrom-Json
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
    $backupDir = Join-Path $targetDir ('cn-oai-' + [Guid]::NewGuid().ToString('N').Substring(0,8))
    New-Item -ItemType Directory -Path $backupDir | Out-Null
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $identity = ([BitConverter]::ToString($sha.ComputeHash($utf8.GetBytes($setup.endpoint + '|' + $setup.groupId)))).Replace('-', '').Substring(0,16).ToLowerInvariant() } finally { $sha.Dispose() }
    $providerId = 'sub2api-cn-oai-' + $identity
    $providerKey = 'sub2api_cn_oai_' + $identity
    $configPath = Join-Path $targetDir 'config.toml'
    $authPath = Join-Path $targetDir 'auth.json'
    $catalogPath = Join-Path $targetDir ('codex-cn-oai-' + $identity + '-models.json')
    foreach ($file in @($configPath, $authPath, $catalogPath, $settingsPath)) {
        $saved = Join-Path $backupDir ([string]$backups.Count + '.bak')
        $existed = Test-Path -LiteralPath $file -PathType Leaf
        if ($existed) { Copy-Item -LiteralPath $file -Destination $saved }
        $backups += [pscustomobject]@{ target = $file; saved = $saved; existed = $existed; before = (Get-SetupHash $file); after = (Get-SetupHash $file) }
    }
    [CNOAISqlite]::Run($db, 'VACUUM INTO ?', @((Join-Path $backupDir 'cc-switch.db'))) | Out-Null
    $restoreScript = @'
$ErrorActionPreference = 'Stop'
if (Get-Process -Name 'cc-switch' -ErrorAction SilentlyContinue) { throw 'Exit CC Switch before restoring this backup.' }
$files = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'restore-files.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$mutex = [Threading.Mutex]::new($false, (Get-Content -LiteralPath (Join-Path $PSScriptRoot 'lock-name.txt') -Raw))
$locked = $false
try {
try { $locked = $mutex.WaitOne(0) } catch [Threading.AbandonedMutexException] { $locked = $true }
if (!$locked) { throw 'An installation or restore is already running.' }
foreach ($file in $files) {
    $current = 'absent'
    if (Test-Path -LiteralPath $file.target -PathType Leaf) {
        $stream = [IO.File]::OpenRead($file.target); $hash = [Security.Cryptography.SHA256]::Create()
        try { $current = ([BitConverter]::ToString($hash.ComputeHash($stream))).Replace('-', '') } finally { $stream.Dispose(); $hash.Dispose() }
    }
    if ($current -ne $file.after) { throw ('Later configuration changes detected. Restore stopped without replacing anything: ' + $file.target) }
}
$currentBackup = Join-Path $PSScriptRoot ('before-restore-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $currentBackup | Out-Null
foreach ($file in $files) {
    if (Test-Path -LiteralPath $file.target -PathType Leaf) { Copy-Item -LiteralPath $file.target -Destination (Join-Path $currentBackup ([string][Array]::IndexOf($files, $file) + '.bak')) }
}
foreach ($file in $files) {
    if ($file.existed) { Copy-Item -LiteralPath $file.saved -Destination $file.target -Force }
    elseif (Test-Path -LiteralPath $file.target) { Remove-Item -LiteralPath $file.target -Force }
    if ([IO.Path]::GetFileName($file.target) -eq 'cc-switch.db') {
        foreach ($suffix in @('-wal','-shm')) {
            $sidecar = $file.target + $suffix
            if (Test-Path -LiteralPath $sidecar) { Remove-Item -LiteralPath $sidecar -Force }
        }
    }
}
Write-Host 'Configuration restored. Restart CC Switch and Codex.'
} finally { if ($locked) { $mutex.ReleaseMutex() }; $mutex.Dispose() }
'@
    Write-SetupFile (Join-Path $backupDir 'restore.ps1') $restoreScript
    # Preserve other Codex settings. Reject multiline TOML rather than risk
    # mistaking a table/key inside a multiline string for actual configuration.
    $existing = if (Test-Path -LiteralPath $configPath) { [IO.File]::ReadAllText($configPath) } else { '' }
    $parsedConfig = Read-CodexToml $existing
    if ($existing.Contains('"""') -or $existing.Contains("'''")) { throw 'This Codex configuration contains multiline TOML. Automatic merging was stopped; no configuration was replaced.' }
    $selectedProfile = $parsedConfig.profile
    $root = $true; $skip = $false; $inProfile = $false; $profileFound = $false; $lines = @()
    $profileModelLines = 'model = ' + (Quote-Toml $model.slug) + "`nmodel_provider = " + (Quote-Toml $providerKey) + "`n"
    if ($model.default_reasoning_level -ne 'none') { $profileModelLines += 'model_reasoning_effort = ' + (Quote-Toml $model.default_reasoning_level) + "`n" }
    foreach ($line in ($existing -split '\r?\n')) {
        if ($line -match '^\s*\[') {
            $root = $false
            $inProfile = $false
            if ($selectedProfile) {
                $profilePattern = '^\s*\[\s*profiles\s*\.\s*["'']?' + [regex]::Escape($selectedProfile) + '["'']?\s*\]\s*(#.*)?$'
                $inProfile = $line -match $profilePattern
            }
            $skip = $line -match ('^\s*\[\s*model_providers\s*\.\s*["'']?' + $providerKey + '["'']?\s*\]\s*(#.*)?$')
            if ($inProfile) { $profileFound = $true; $lines += $line; $lines += $profileModelLines.TrimEnd(); continue }
        }
        if ($skip) { continue }
        if ($root -and $line -match '^\s*["'']?(model|review_model|model_provider|model_reasoning_effort|model_catalog_json)["'']?\s*=') { continue }
        if ($inProfile -and $line -match '^\s*["'']?(model|model_provider|model_reasoning_effort)["'']?\s*=') { continue }
        $lines += $line
    }
    if ($selectedProfile -and !$profileFound) { throw 'The selected profile uses a configuration form that cannot be merged automatically. No configuration was replaced.' }
    $effortLine = if ($model.default_reasoning_level -ne 'none') { 'model_reasoning_effort = ' + (Quote-Toml $model.default_reasoning_level) + "`n" } else { '' }
    $config = 'model = ' + (Quote-Toml $model.slug) + "`n" +
        'review_model = ' + (Quote-Toml $model.slug) + "`n" +
        'model_provider = ' + (Quote-Toml $providerKey) + "`n" + $effortLine +
        'model_catalog_json = ' + (Quote-Toml $catalogPath.Replace('\','/')) + "`n" +
        ($lines -join "`n") + "`n[model_providers.$providerKey]`n" +
        'name = ' + (Quote-Toml $setup.providerName) + "`n" +
        'base_url = ' + (Quote-Toml $setup.endpoint) + "`n" +
        "wire_api = `"responses`"`nrequires_openai_auth = true`n"
    $validated = Read-CodexToml $config $response.Content
    if ($selectedProfile -and $validated.profile -ne $selectedProfile) { throw 'The selected profile was not preserved.' }
    $auth = @{ OPENAI_API_KEY = $setup.apiKey } | ConvertTo-Json -Compress
    $providerConfig = @{ auth = @{ OPENAI_API_KEY = $setup.apiKey }; config = $config } | ConvertTo-Json -Depth 20 -Compress
    $settings | Add-Member -NotePropertyName currentProviderCodex -NotePropertyValue $providerId -Force
    $settings | Add-Member -NotePropertyName codexConfigDir -NotePropertyValue $targetDir -Force
    $meta = @{ commonConfigEnabled = $false } | ConvertTo-Json -Compress

    $contents = @($config, $auth, $response.Content, ($settings | ConvertTo-Json -Depth 100))
    foreach ($index in 0..3) {
        $stage = Join-Path $backupDir ('install-' + $index + '.tmp')
        Write-SetupFile $stage $contents[$index]
        $backups[$index].after = Get-SetupHash $stage
    }
    # Write-ahead file journal plus an atomic SQLite commit marker distinguishes
    # interrupted writes from a committed install on the next execution.
    Write-SetupFile $journalPath (@{ id = $operationId; files = $backups } | ConvertTo-Json -Depth 20)

    # Register a complete provider, not a normal deeplink (which discards the
    # catalog path). Existing providers and common configuration stay intact.
    [CNOAISqlite]::Run($db, 'BEGIN IMMEDIATE', @()) | Out-Null
    $transaction = $true
    if (@(Get-Process -Name 'cc-switch' -ErrorAction SilentlyContinue).Count) { throw 'CC Switch was opened during setup. Exit it and retry.' }
    [CNOAISqlite]::Run($db, 'INSERT INTO sub2api_cn_oai_operations (id) VALUES (?)', @($operationId)) | Out-Null
    [CNOAISqlite]::Run($db, "UPDATE providers SET is_current = 0 WHERE app_type = 'codex'", @()) | Out-Null
    [CNOAISqlite]::Run($db, "INSERT INTO providers (id,app_type,name,settings_config,meta,is_current) VALUES (?,'codex',?,?,?,1) ON CONFLICT(id,app_type) DO UPDATE SET name=excluded.name,settings_config=excluded.settings_config,meta=excluded.meta,is_current=1", @($providerId, [string]$setup.providerName, $providerConfig, $meta)) | Out-Null
    Write-SetupFile $catalogPath $response.Content
    Write-SetupFile $configPath $config
    Write-SetupFile $authPath $auth
    Write-SetupFile $settingsPath ($settings | ConvertTo-Json -Depth 100)
    [CNOAISqlite]::Run($db, 'COMMIT', @()) | Out-Null
    $transaction = $false
    $committed = $true
    Remove-Item -LiteralPath $journalPath -Force
    [CNOAISqlite]::Run($db, 'PRAGMA wal_checkpoint(TRUNCATE)', @()) | Out-Null
    [CNOAISqlite]::sqlite3_close($db) | Out-Null
    $db = [IntPtr]::Zero
    $restoreFiles = @($backups) + @([pscustomobject]@{ target = $dbPath; saved = (Join-Path $backupDir 'cc-switch.db'); existed = $true; after = (Get-SetupHash $dbPath) })
    foreach ($suffix in @('-wal','-shm')) {
        $sidecar = $dbPath + $suffix
        $restoreFiles += [pscustomobject]@{ target = $sidecar; saved = ''; existed = $false; after = (Get-SetupHash $sidecar) }
    }
    Write-SetupFile (Join-Path $backupDir 'restore-files.json') (ConvertTo-Json -InputObject $restoreFiles -Depth 10)
    Write-SetupFile (Join-Path $backupDir 'lock-name.txt') ('Local\Sub2APICNOAI-' + $lockId)
    Write-Host ('Configured ' + $model.slug + '; ' + $catalog.models.Count + ' models installed.')
    Write-Host ('Backup: ' + $backupDir)
    Write-Host 'Restart Codex to load the model catalog. CC Switch can now switch back to this provider.'
} catch {
    if ($transaction) { [CNOAISqlite]::Run($db, 'ROLLBACK', @()) | Out-Null; $transaction = $false }
    if (!$committed) {
        Restore-SetupFiles $backups
        if ($backups.Count -and $journalPath -and (Test-Path -LiteralPath $journalPath)) { Remove-Item -LiteralPath $journalPath -Force }
    }
    Write-Host ('Setup failed: ' + $_.Exception.Message) -ForegroundColor Red
} finally {
    if ($db -ne [IntPtr]::Zero) { [CNOAISqlite]::sqlite3_close($db) | Out-Null }
    if ($lockHeld) { $setupMutex.ReleaseMutex() }
    if ($setupMutex) { $setupMutex.Dispose() }
    if ($committed -and $ccExecutable -and (Test-Path -LiteralPath $ccExecutable)) {
        try { Start-Process -FilePath $ccExecutable -WindowStyle Hidden | Out-Null } catch { Write-Host 'Configuration saved. Open CC Switch manually.' }
    }
}
if (!$committed) { exit 1 }
