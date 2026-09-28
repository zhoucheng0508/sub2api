import { afterEach, describe, expect, it } from 'vitest'
import { mkdtempSync, readFileSync, writeFileSync, mkdirSync, rmSync, readdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawn, spawnSync } from 'node:child_process'
import { buildCnOaiSetupScript, CN_OAI_SETUP_FILENAME, isCnOaiGroup } from '../cnOaiSetup'

const input = { baseUrl: 'https://example.com', apiKey: "sk-test-'&%secret", providerName: '测试服务', groupId: 91 }
const directories: string[] = []
afterEach(() => { directories.splice(0).forEach(p => rmSync(p, { recursive: true, force: true })) })

describe('CN OAI setup', () => {
  it('opts in only the named OpenAI group', () => {
    expect(isCnOaiGroup({ name: ' 国模 Oai ', platform: 'openai' })).toBe(true)
    for (const group of [null, { name: '国模OAI', platform: 'composite' }, { name: 'OpenAI', platform: 'openai' }, { name: '国模OAI backup', platform: 'openai' }]) {
      expect(isCnOaiGroup(group)).toBe(false)
    }
  })

  it('uses a double-click launcher with a data-only credential payload', () => {
    const script = buildCnOaiSetupScript(input)
    expect(CN_OAI_SETUP_FILENAME).toMatch(/\.cmd$/)
    expect(script.startsWith('@echo off\r\n')).toBe(true)
    expect(script).toContain('DisableDelayedExpansion')
    expect(script).toContain('pause\r\nexit /b %SUB2API_SETUP_EXIT%')
    expect(script).not.toContain(input.apiKey)
    const payload = JSON.parse(Buffer.from(script.match(/\$setupPayload = '([^']+)'/)![1], 'base64').toString('utf8'))
    expect(payload.apiKey).toBe(input.apiKey)
    expect(payload.manifestUrl).toBe('https://example.com/v1/models?client_version=0.147.0')
    expect(payload.providerName).toContain(input.providerName)
    expect(() => buildCnOaiSetupScript({ ...input, baseUrl: 'file:///tmp' })).toThrow()
  })
})

// Exercise the exact downloadable CMD, PowerShell 5.1 and Windows SQLite. All
// file paths are temporary, network is a fixture, and process control is mocked
// before the production script runs so installed applications cannot be touched.
describe.skipIf(process.platform !== 'win32')('Windows setup execution', () => {
  const completeModel = (slug: string, efforts: string[], defaultEffort: string) => ({
    slug, display_name: slug, description: 'Local test model', base_instructions: 'You are Codex.', default_reasoning_level: defaultEffort,
    supported_reasoning_levels: efforts.map(effort => ({ effort, description: effort })),
    shell_type: 'unified_exec', visibility: 'list', supported_in_api: true, priority: 50,
    additional_speed_tiers: [], service_tiers: [], default_service_tier: null, availability_nux: null, upgrade: null,
    model_messages: { instructions_template: 'test', instructions_variables: null, approvals: null, collaboration_modes: null, auto_review: null, permissions: null, multi_agent: null, token_budget: null, guardian_v2: null },
    include_skills_usage_instructions: false, include_plugin_usage_instructions: false, include_apps_usage_instructions: false,
    supports_reasoning_summary_parameter: true, default_reasoning_summary: 'auto', support_verbosity: false,
    default_verbosity: null, apply_patch_tool_type: null, web_search_tool_type: 'text',
    truncation_policy: { mode: 'bytes', limit: 10000 }, supports_image_detail_original: true,
    supports_parallel_tool_calls: false, context_window: 272000, max_context_window: 272000,
    auto_compact_token_limit: null, comp_hash: null, effective_context_window_percent: 95,
    experimental_supported_tools: [], input_modalities: ['text', 'image'], supports_search_tool: false,
    use_responses_lite: false, node_repl_auto_review_required: false, node_repl_disabled: false,
    auto_review_model_override: null, model_specialty: null, tool_mode: null, multi_agent_version: null
  })

  function fixture() {
    const dir = mkdtempSync(join(tmpdir(), 'cn-oai-test-'))
    directories.push(dir)
    const cc = join(dir, 'cc'); const codex = join(dir, '用户 Codex & files')
    mkdirSync(cc); mkdirSync(codex)
    const settingsPath = join(cc, 'settings.json')
    writeFileSync(settingsPath, JSON.stringify({ currentProviderCodex: 'old', codexConfigDir: codex, customPreference: 'keep' }))
    const originalConfig = 'model = "old-model"\nmodel_provider = "old"\nmodel_reasoning_effort = "xhigh"\n[features]\ngoals = true\n[model_providers.old]\nname = "Existing provider"\nbase_url = "https://old.example/v1"\nwire_api = "responses"\n'
    writeFileSync(join(codex, 'config.toml'), originalConfig)
    writeFileSync(join(codex, 'auth.json'), '{"OPENAI_API_KEY":"old-key"}')
    const catalog = { models: [
      completeModel('text-model', ['none'], 'none'),
      completeModel('glm-5.3', ['low', 'medium', 'high'], 'medium')
    ] }
    writeFileSync(join(dir, 'catalog.json'), JSON.stringify(catalog))
    const dbFile = join(cc, 'cc-switch.db')
    const init = spawnSync(process.execPath, ['--no-warnings', '-e', `
      const {DatabaseSync}=require('node:sqlite'); const db=new DatabaseSync(process.argv[1]);
      db.exec("CREATE TABLE providers(id TEXT,app_type TEXT,name TEXT,settings_config TEXT,meta TEXT,is_current INTEGER,PRIMARY KEY(id,app_type)); CREATE TABLE proxy_config(app_type TEXT,enabled INTEGER,live_takeover_active INTEGER); INSERT INTO proxy_config VALUES('codex',0,0); INSERT INTO providers VALUES('old','codex','Old','{}','{}',1);");
      db.close();`, dbFile], { encoding: 'utf8' })
    expect(init.status, init.stderr).toBe(0)
    const quote = (s: string) => `'${s.replace(/'/g, "''")}'`
    const mocks = `
function Get-Process { @() }
function Start-Process { }
function Read-CodexToml { param($content) return [pscustomobject]@{} }
function Invoke-WebRequest {
  param($Uri,$Headers,[switch]$UseBasicParsing,$TimeoutSec,$MaximumRedirection)
  if ($Headers.Authorization -ne ${quote('Bearer ' + input.apiKey)}) { throw 'Wrong authorization' }
  return [pscustomobject]@{ Content = [IO.File]::ReadAllText(${quote(join(dir, 'catalog.json'))}) }
}
`
    const run = (change: (s: string) => string = s => s, model?: string) => {
      const script = change(buildCnOaiSetupScript({ ...input, model }).replace("    Write-Host 'Preparing the dedicated CN OAI configuration...'", mocks + "\n    Write-Host 'Preparing the dedicated CN OAI configuration...'"))
      const file = join(dir, CN_OAI_SETUP_FILENAME)
      writeFileSync(file, script)
      return spawnSync('cmd.exe', ['/d', '/c', file], { encoding: 'utf8', input: '\r\n', timeout: 30000, windowsHide: true,
        env: { ...process.env, SUB2API_CC_SWITCH_DIR: cc, CODEX_HOME: codex } })
    }
    const providers = () => {
      const result = spawnSync(process.execPath, ['--no-warnings', '-e', `const {DatabaseSync}=require('node:sqlite'); const db=new DatabaseSync(process.argv[1]); console.log(JSON.stringify(db.prepare('SELECT * FROM providers ORDER BY id').all())); db.close();`, dbFile], { encoding: 'utf8' })
      expect(result.status).toBe(0)
      return JSON.parse(result.stdout) as Array<{ id: string; settings_config: string; is_current: number }>
    }
    return { dir, cc, codex, run, providers, originalConfig, settingsPath }
  }

  it('installs and selects a persistent provider, preserving other settings on repeated runs', () => {
    const f = fixture()
    for (let i = 0; i < 2; i++) {
      const result = f.run()
      expect(result.status, result.stdout + result.stderr).toBe(0)
      const config = readFileSync(join(f.codex, 'config.toml'), 'utf8')
      expect(config).toContain('model = "glm-5.3"')
      expect(config).toContain('model_reasoning_effort = "medium"')
      expect(config).toContain('[features]\ngoals = true')
      expect(config).toContain('[model_providers.old]')
      expect(config.match(/^model_catalog_json = /gm)).toHaveLength(1)
      expect(config.match(/^\[model_providers.sub2api_cn_oai_/gm)).toHaveLength(1)
      const catalogPath = JSON.parse(config.match(/^model_catalog_json = (.+)$/m)![1])
      expect(JSON.parse(readFileSync(catalogPath, 'utf8')).models[1].input_modalities).toContain('image')
      const rows = f.providers()
      expect(rows).toHaveLength(2)
      expect(rows.find(x => x.id === 'old')!.settings_config).toBe('{}')
      const selected = rows.find(x => x.is_current === 1)!
      expect(JSON.parse(selected.settings_config).config).toBe(config)
      expect(JSON.parse(selected.settings_config).auth.OPENAI_API_KEY).toBe(input.apiKey)
      const settings = JSON.parse(readFileSync(f.settingsPath, 'utf8'))
      expect(settings.currentProviderCodex).toBe(selected.id)
      expect(settings.customPreference).toBe('keep')
    }
    expect(readdirSync(f.codex).filter(p => p.startsWith('cn-oai-'))).toHaveLength(2)
  }, 60000)

  it('omits forced reasoning for none-only models', () => {
    const f = fixture(); const result = f.run(undefined, 'text-model')
    expect(result.status, result.stdout + result.stderr).toBe(0)
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).not.toMatch(/^model_reasoning_effort/m)
  }, 30000)

  it('leaves configuration intact on catalog failure', () => {
    const f = fixture()
    writeFileSync(join(f.dir, 'catalog.json'), '{"models":[]}')
    const result = f.run()
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('empty model catalog')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    expect(f.providers()).toHaveLength(1)
  }, 30000)

  it('rolls back database and files when installation fails midway', () => {
    const f = fixture()
    const result = f.run(s => s.replace('Write-SetupFile $settingsPath', "throw 'Fixture write failure'\r\n    Write-SetupFile $settingsPath"))
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('Fixture write failure')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    expect(readFileSync(join(f.codex, 'auth.json'), 'utf8')).toBe('{"OPENAI_API_KEY":"old-key"}')
    expect(f.providers()).toEqual([expect.objectContaining({ id: 'old', is_current: 1 })])
    expect(JSON.parse(readFileSync(f.settingsPath, 'utf8')).currentProviderCodex).toBe('old')
  }, 30000)

  it('preserves selected profile and context settings when the installed parser supports profiles', () => {
    const f = fixture()
    writeFileSync(join(f.codex, 'config.toml'), 'profile = "work"\nmodel_context_window = 128000\nmodel_auto_compact_token_limit = 100000\n[profiles."work"]\nmodel = "old-model"\nmodel_provider = "old"\nsandbox_mode = "read-only"\napproval_policy = "never"\n[model_providers.old]\nname = "Old"\nbase_url = "https://old.example/v1"\nwire_api = "responses"\n')
    const result = f.run(s => s.replace('function Read-CodexToml { param($content) return [pscustomobject]@{} }', 'function Read-CodexToml { param($content) return [pscustomobject]@{profile="work"} }'))
    expect(result.status, result.stdout + result.stderr).toBe(0)
    const config = readFileSync(join(f.codex, 'config.toml'), 'utf8')
    expect(config).toContain('profile = "work"')
    expect(config).toContain('model_context_window = 128000')
    expect(config).toContain('model_auto_compact_token_limit = 100000')
    expect(config).toContain('sandbox_mode = "read-only"')
    expect(config).toContain('approval_policy = "never"')
    expect(config).toMatch(/\[profiles\."work"\]\nmodel = "glm-5.3"/)
    const rerun = f.run(s => s.replace('function Read-CodexToml { param($content) return [pscustomobject]@{} }', 'function Read-CodexToml { param($content) return [pscustomobject]@{profile="work"} }'))
    expect(rerun.status, rerun.stdout + rerun.stderr).toBe(0)
  }, 60000)

  it('validates quoted provider tables with the real installed Codex parser', () => {
    const f = fixture()
    const installed = f.run()
    expect(installed.status).toBe(0)
    const configPath = join(f.codex, 'config.toml')
    const config = readFileSync(configPath, 'utf8').replace(/\[model_providers\.(sub2api_cn_oai_\w+)\]/, '[model_providers."$1"]')
    writeFileSync(configPath, config)
    const result = f.run(s => s.replace('function Read-CodexToml { param($content) return [pscustomobject]@{} }', ''))
    expect(result.status, result.stdout + result.stderr).toBe(0)
    expect(readFileSync(configPath, 'utf8').match(/\[model_providers\.["']?sub2api_cn_oai_/g)).toHaveLength(1)
  }, 60000)

  it('uses the real Codex parser to reject an incomplete model catalog', () => {
    const f = fixture()
    const catalog = JSON.parse(readFileSync(join(f.dir, 'catalog.json'), 'utf8'))
    delete catalog.models[0].base_instructions
    writeFileSync(join(f.dir, 'catalog.json'), JSON.stringify(catalog))
    const result = f.run(s => s.replace('function Read-CodexToml { param($content) return [pscustomobject]@{} }', ''))
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('did not load every model')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    expect(f.providers()).toHaveLength(1)
  }, 60000)

  it('rejects illegal capability values before installation', () => {
    for (const invalid of [
      { slug: 'bad', default_reasoning_level: 'garbage', supported_reasoning_levels: [{ effort: 'garbage' }], input_modalities: ['text'] },
      { slug: 'bad', default_reasoning_level: 'high', supported_reasoning_levels: [{ effort: 'high' }], input_modalities: ['banana'] },
      { slug: 'bad', default_reasoning_level: 'high', supported_reasoning_levels: [{ effort: 'high' }, { effort: 'high' }], input_modalities: ['text'] }
    ]) {
      const f = fixture(); writeFileSync(join(f.dir, 'catalog.json'), JSON.stringify({ models: [invalid] }))
      const result = f.run()
      expect(result.status, result.stdout + result.stderr).toBe(1)
      expect(result.stdout).toContain('invalid')
      expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    }
  }, 30000)

  it('does not terminate a running CC Switch', () => {
    const f = fixture()
    const result = f.run(s => s.replace('function Get-Process { @() }', 'function Get-Process { @([pscustomobject]@{ Id = 1 }) }'))
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('Save your edits and exit CC Switch')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
  }, 30000)

  it('recovers files from a process interruption before database commit', () => {
    const f = fixture()
    const crash = f.run(s => s.replace("[CNOAISqlite]::Run($db, 'COMMIT', @())", "[Environment]::Exit(73)\r\n    [CNOAISqlite]::Run($db, 'COMMIT', @())"))
    expect(crash.status).toBe(73)
    expect(f.providers()).toEqual([expect.objectContaining({ id: 'old', is_current: 1 })])
    const result = f.run(s => s.replace("  if ($Headers.Authorization", "  throw 'Network unavailable after recovery'\r\n  if ($Headers.Authorization"))
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('Network unavailable after recovery')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    expect(JSON.parse(readFileSync(f.settingsPath, 'utf8')).currentProviderCodex).toBe('old')
  }, 30000)

  it('refuses concurrent installation before touching configuration', async () => {
    const f = fixture()
    const holder = spawn('powershell.exe', ['-NoProfile', '-Command', `$p=[IO.Path]::GetFullPath('${f.cc.replace(/'/g, "''")}');$h=[Security.Cryptography.SHA256]::Create();$id=([BitConverter]::ToString($h.ComputeHash([Text.Encoding]::UTF8.GetBytes($p.ToLowerInvariant())))).Replace('-','');$m=[Threading.Mutex]::new($false,('Local'+[char]92+'Sub2APICNOAI-'+$id));$m.WaitOne()|Out-Null;[Console]::WriteLine('LOCKED');try{Start-Sleep -Seconds 20}finally{$m.ReleaseMutex();$m.Dispose();$h.Dispose()}`], { windowsHide: true })
    try {
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error('Lock fixture timed out')), 5000)
        holder.stdout.once('data', () => { clearTimeout(timer); resolve() })
        holder.once('error', error => { clearTimeout(timer); reject(error) })
      })
      const result = f.run()
      expect(result.status, result.stdout + result.stderr).toBe(1)
      expect(result.stdout + result.stderr).toContain('Another CN OAI setup is running')
      expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
      expect(f.providers()).toHaveLength(1)
    } finally {
      await new Promise<void>(resolve => { holder.once('exit', () => resolve()); holder.kill() })
    }
  }, 15000)

  it('refuses old full restores after another client provider was added', () => {
    const f = fixture(); expect(f.run().status).toBe(0)
    const dbPath = join(f.cc, 'cc-switch.db')
    const add = spawnSync(process.execPath, ['--no-warnings', '-e', `const {DatabaseSync}=require('node:sqlite');const db=new DatabaseSync(process.argv[1]);db.exec("INSERT INTO providers VALUES('later-claude','claude','Later','{}','{}',1)");db.close();`, dbPath], { encoding: 'utf8' })
    expect(add.status).toBe(0)
    const backup = readdirSync(f.codex).find(p => p.startsWith('cn-oai-'))!
    const restorePath = join(f.codex, backup, 'restore.ps1')
    writeFileSync(restorePath, 'function Get-Process { @() }\n' + readFileSync(restorePath, 'utf8'))
    const restored = spawnSync('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', restorePath], { encoding: 'utf8', timeout: 15000, windowsHide: true })
    expect(restored.status).not.toBe(0)
    expect(restored.stderr).toContain('Later configuration changes detected')
    expect(f.providers().find(x => x.id === 'later-claude')).toBeDefined()
  }, 30000)

  it('fails without writes on HTTP errors and requests no redirects', () => {
    const f = fixture()
    const result = f.run(s => s.replace("  if ($Headers.Authorization", "  if ($MaximumRedirection -ne 0) { throw 'Unexpected redirect policy' }\r\n  throw 'HTTP 502 fixture'\r\n  if ($Headers.Authorization"))
    expect(result.status).toBe(1)
    expect(result.stdout).toContain('HTTP 502 fixture')
    expect(readFileSync(join(f.codex, 'config.toml'), 'utf8')).toBe(f.originalConfig)
    expect(f.providers()).toHaveLength(1)
  }, 30000)
})
