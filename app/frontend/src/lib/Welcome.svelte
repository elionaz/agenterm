<!-- First run (no projects yet): agents found, notification permission,
     the CLI, and adding the first project. Nothing here is required: it
     only explains and offers shortcuts. -->
<script lang="ts">
  import { keyLabel } from './platform'
  import { onMount } from 'svelte'
  import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
  import { api, type OnboardingInfo, type Profile } from './api'
  import { t } from './i18n.svelte'

  interface Props {
    profiles: Profile[]
    onAddProject: () => void
    onOpenSettings: () => void
    /** Re-detects installed agents (updates profiles). */
    onRescan: () => Promise<void>
  }

  let { profiles, onAddProject, onOpenSettings, onRescan }: Props = $props()

  let info = $state<OnboardingInfo | null>(null)
  let rescanning = $state(false)
  let asking = $state(false)
  let copied = $state(false)

  const installed = $derived(profiles.filter((p) => p.installed && p.id !== 'shell'))
  /** Quoted for the shell: the bundled CLI lives in /Applications/agenterm.app. */
  const linkCmd = $derived(
    info?.bundledCli
      ? `sudo mkdir -p /usr/local/bin && sudo ln -sf '${info.bundledCli.replaceAll("'", "'\\''")}' /usr/local/bin/agenterm`
      : '',
  )

  async function load() {
    try {
      info = await api.onboarding()
    } catch {
      info = null
    }
  }

  onMount(() => {
    void load()
    // Coming back from System Settings: show the new permission.
    const onFocus = () => void load()
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  })

  async function rescan() {
    rescanning = true
    try {
      await onRescan()
    } finally {
      rescanning = false
    }
  }

  async function allow() {
    asking = true
    try {
      const s = await api.requestNotifications()
      if (info) info = { ...info, notifications: s }
    } finally {
      asking = false
    }
  }

  async function copy() {
    await ClipboardSetText(linkCmd)
    copied = true
    setTimeout(() => (copied = false), 1500)
  }
</script>

<div class="welcome">
  <h2>{t('welcome.title')}</h2>
  <p class="sub">{t('welcome.subtitle')}</p>

  <section>
    <h3><span class="dot" class:ok={installed.length > 0}></span>{t('welcome.agents')}</h3>
    {#if installed.length}
      <p>{t('welcome.agentsFound', { n: installed.length })}</p>
      <ul class="agents">
        {#each installed as p (p.id)}<li title={p.path}>{p.name || p.id}</li>{/each}
      </ul>
      <p class="hint">{t('welcome.agentsHooks')}</p>
    {:else}
      <p>{t('welcome.agentsNone')}</p>
    {/if}
    <div class="actions">
      <button disabled={rescanning} onclick={rescan}>{t('welcome.rescan')}</button>
      <button class="linkish" onclick={onOpenSettings}>{t('welcome.chooseAgents')} <kbd>{keyLabel('⌘,')}</kbd></button>
    </div>
  </section>

  {#if info}
    <section>
      <h3><span class="dot" class:ok={info.notifications === 'authorized'}></span>{t('welcome.notifications')}</h3>
      {#if info.notifications === 'authorized'}
        <p>{t('welcome.notifyOn')}</p>
      {:else if info.notifications === 'denied'}
        <p>{t('welcome.notifyDenied')}</p>
        <div class="actions">
          <button onclick={() => void api.openNotificationSettings()}>{t('welcome.notifyOpen')}</button>
        </div>
      {:else if info.notifications === 'off'}
        <p>{t('welcome.notifyOff')}</p>
      {:else if info.notifications === 'unsupported'}
        <p>{t('welcome.notifyUnsupported')}</p>
      {:else}
        <p>{t('welcome.notifyAsk')}</p>
        <div class="actions">
          <button disabled={asking} onclick={allow}>{t('welcome.notifyAllow')}</button>
        </div>
      {/if}
    </section>

    {#if info.cliPath || linkCmd}
      <section>
        <h3><span class="dot" class:ok={!!info.cliPath}></span>{t('welcome.cli')}</h3>
        {#if info.cliPath}
          <p>{t('welcome.cliOn', { path: info.cliPath })}</p>
        {:else}
          <p>{t('welcome.cliOff')}</p>
          <div class="cmd">
            <code>{linkCmd}</code>
            <button onclick={copy}>{copied ? t('welcome.copied') : t('welcome.copy')}</button>
          </div>
        {/if}
      </section>
    {/if}
  {/if}

  <button class="primary" onclick={onAddProject}>{t('welcome.start')}</button>
  <p class="hint">{t('welcome.startHint')}</p>
</div>

<style>
  .welcome {
    width: min(520px, calc(100% - 48px));
    max-height: calc(100% - 48px);
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: 10px;
    text-align: left;
    color: var(--fg-dim);
  }
  h2 {
    margin: 0;
    font-size: 18px;
    font-weight: 600;
    color: var(--fg);
  }
  .sub {
    margin: 0 0 6px;
  }
  section {
    padding: 12px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--sidebar-bg);
  }
  h3 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 6px;
    font-size: 13px;
    font-weight: 600;
    color: var(--fg);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--amber);
  }
  .dot.ok {
    background: var(--green);
  }
  p {
    margin: 0 0 6px;
  }
  .hint {
    font-size: 12px;
    color: var(--muted);
  }
  .agents {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 0 0 8px;
    padding: 0;
    list-style: none;
  }
  .agents li {
    padding: 2px 8px;
    border: 1px solid var(--border-strong);
    border-radius: 10px;
    font-size: 12px;
    color: var(--fg);
  }
  .actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  button {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
    color: var(--fg);
    background: var(--bg);
  }
  button:hover:not(:disabled) {
    background: var(--hover);
  }
  .linkish {
    border: none;
    background: none;
    color: var(--blue);
    padding: 4px 0;
  }
  kbd {
    font-family: var(--mono);
    font-size: 11px;
    padding: 1px 5px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
  }
  .cmd {
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }
  code {
    flex: 1;
    min-width: 0;
    padding: 6px 8px;
    border-radius: 6px;
    background: var(--bg);
    border: 1px solid var(--border);
    font: 11px var(--mono);
    word-break: break-all;
    user-select: text;
  }
  .primary {
    align-self: flex-start;
    margin-top: 4px;
    padding: 7px 16px;
    font-weight: 600;
    background: var(--accent-bg);
  }
</style>
