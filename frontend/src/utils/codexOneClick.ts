import { normalizeV1Endpoint } from '@/utils/ccswitchImport'

export const DEFAULT_CODEX_MODEL = 'gpt-5.6'

export type CodexOperatingSystem = 'macos' | 'windows' | 'linux'
export type CodexAuthMode = 'legacy' | 'api-key'

export interface CodexConfigFile {
  path: string
  content: string
}

export function isCodexOneClickEligible(key: {
  status: string
  key?: string | null
}): boolean {
  return key.status === 'active' && typeof key.key === 'string' && key.key.trim().length > 0
}

function escapeTomlString(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/[\r\n]/g, '')
}

export function buildCodexConfigFiles(
  baseUrl: string,
  apiKey: string,
  authMode: CodexAuthMode = 'legacy',
  websocket = false
): CodexConfigFile[] {
  const providerAuth = authMode === 'api-key'
    ? 'requires_openai_auth = false\nhttp_headers = { "x-openai-actor-authorization" = "local-image-extension" }'
    : 'requires_openai_auth = true'
  const websocketProvider = websocket ? '\nsupports_websockets = true' : ''
  const websocketFeature = websocket ? 'responses_websockets_v2 = true\n' : ''
  const normalizedBaseUrl = escapeTomlString(normalizeV1Endpoint(baseUrl))

  return [
    {
      path: 'config.toml',
      content: `model_provider = "OpenAI"
model = "${DEFAULT_CODEX_MODEL}"
review_model = "${DEFAULT_CODEX_MODEL}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${normalizedBaseUrl}"
wire_api = "responses"${websocketProvider}
${providerAuth}

[features]
${websocketFeature}goals = true`
    },
    {
      path: 'auth.json',
      content: JSON.stringify({ OPENAI_API_KEY: apiKey }, null, 2)
    }
  ]
}

function utf8ToBase64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach((byte) => { binary += String.fromCharCode(byte) })
  return btoa(binary)
}

function buildUnixScript(files: CodexConfigFile[], os: Exclude<CodexOperatingSystem, 'windows'>): string {
  const config = utf8ToBase64(files[0].content)
  const auth = utf8ToBase64(files[1].content)
  const decodeFlag = os === 'macos' ? '-D' : '--decode'
  return `#!/usr/bin/env sh
set -eu

target_dir="\${CODEX_HOME:-\${HOME}/.codex}"
mkdir -p "$target_dir"
backup_dir="$(mktemp -d "$target_dir/sub2api-backup-XXXXXXXX")"

for file in config.toml auth.json; do
  if [ -f "$target_dir/$file" ]; then
    cp -p "$target_dir/$file" "$backup_dir/$file"
  else
    : > "$backup_dir/$file.absent"
  fi
done

cat > "$backup_dir/restore.sh" <<'RESTORE'
#!/usr/bin/env sh
set -eu
backup_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
target_dir="\${CODEX_HOME:-\${HOME}/.codex}"
for file in config.toml auth.json; do
  if [ -f "$backup_dir/$file" ]; then
    cp -p "$backup_dir/$file" "$target_dir/$file"
  elif [ -f "$backup_dir/$file.absent" ]; then
    rm -f "$target_dir/$file"
  fi
done
printf '%s\\n' "Codex configuration restored from $backup_dir"
RESTORE
chmod 700 "$backup_dir/restore.sh"

printf '%s' '${config}' | base64 ${decodeFlag} > "$target_dir/config.toml.tmp"
printf '%s' '${auth}' | base64 ${decodeFlag} > "$target_dir/auth.json.tmp"
chmod 600 "$target_dir/config.toml.tmp" "$target_dir/auth.json.tmp"
mv -f "$target_dir/config.toml.tmp" "$target_dir/config.toml"
mv -f "$target_dir/auth.json.tmp" "$target_dir/auth.json"

printf '%s\\n' "Codex configured. Backup: $backup_dir"
printf '%s\\n' "Rollback: $backup_dir/restore.sh"
`
}

function buildWindowsScript(files: CodexConfigFile[]): string {
  const config = utf8ToBase64(files[0].content)
  const auth = utf8ToBase64(files[1].content)
  return `param()
$ErrorActionPreference = 'Stop'
$targetDir = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $HOME '.codex' }
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
do {
  $backupDir = Join-Path $targetDir ("sub2api-backup-" + [IO.Path]::GetRandomFileName())
} while (Test-Path -LiteralPath $backupDir)
New-Item -ItemType Directory -Path $backupDir | Out-Null

foreach ($file in @('config.toml', 'auth.json')) {
  $source = Join-Path $targetDir $file
  if (Test-Path -LiteralPath $source -PathType Leaf) {
    Copy-Item -LiteralPath $source -Destination (Join-Path $backupDir $file) -Force
  } else {
    New-Item -ItemType File -Path (Join-Path $backupDir "$file.absent") -Force | Out-Null
  }
}

$restore = @'
$ErrorActionPreference = 'Stop'
$backupDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$targetDir = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $HOME '.codex' }
foreach ($file in @('config.toml', 'auth.json')) {
  $saved = Join-Path $backupDir $file
  $target = Join-Path $targetDir $file
  if (Test-Path -LiteralPath $saved -PathType Leaf) {
    Copy-Item -LiteralPath $saved -Destination $target -Force
  } elseif (Test-Path -LiteralPath (Join-Path $backupDir "$file.absent")) {
    Remove-Item -LiteralPath $target -Force -ErrorAction SilentlyContinue
  }
}
Write-Host "Codex configuration restored from $backupDir"
'@
[IO.File]::WriteAllText((Join-Path $backupDir 'restore.ps1'), $restore, [Text.UTF8Encoding]::new($false))

[IO.File]::WriteAllBytes((Join-Path $targetDir 'config.toml.tmp'), [Convert]::FromBase64String('${config}'))
[IO.File]::WriteAllBytes((Join-Path $targetDir 'auth.json.tmp'), [Convert]::FromBase64String('${auth}'))
Move-Item -LiteralPath (Join-Path $targetDir 'config.toml.tmp') -Destination (Join-Path $targetDir 'config.toml') -Force
Move-Item -LiteralPath (Join-Path $targetDir 'auth.json.tmp') -Destination (Join-Path $targetDir 'auth.json') -Force

Write-Host "Codex configured. Backup: $backupDir"
Write-Host "Rollback: powershell -ExecutionPolicy Bypass -File \`"$(Join-Path $backupDir 'restore.ps1')\`""
`
}

export function buildCodexSetupScript(
  os: CodexOperatingSystem,
  baseUrl: string,
  apiKey: string
): string {
  const files = buildCodexConfigFiles(baseUrl, apiKey, 'legacy')
  return os === 'windows' ? buildWindowsScript(files) : buildUnixScript(files, os)
}

export function buildCodexSetupScriptPreview(
  os: CodexOperatingSystem,
  baseUrl: string
): string {
  const files = buildCodexConfigFiles(baseUrl, '<API_KEY>', 'legacy')
  const script = os === 'windows' ? buildWindowsScript(files) : buildUnixScript(files, os)

  return script
    .replace(utf8ToBase64(files[0].content), '<CONFIG_TOML_BASE64_REDACTED>')
    .replace(utf8ToBase64(files[1].content), '<AUTH_JSON_BASE64_REDACTED>')
}

export function getCodexSetupFilename(os: CodexOperatingSystem): string {
  return os === 'windows' ? 'sub2api-codex-windows.ps1' : `sub2api-codex-${os}.sh`
}
