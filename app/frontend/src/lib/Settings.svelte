<script lang="ts">
  import { onMount } from 'svelte'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import {
    api,
    errorMessage,
    joinArgs,
    launchable,
    splitArgs,
    type Profile,
    type ProfileEntry,
    type Settings,
  } from './api'
  import { langSetting, setLangSetting, systemLang, t, th, type LangSetting } from './i18n.svelte'

  interface Props {
    profiles: Profile[]
    onProfiles: (ps: Profile[]) => void
    /** Settings were saved (the app keeps git.default_mode). */
    onSettings?: (s: Settings) => void
    onClose: () => void
  }

  let { profiles, onProfiles, onSettings, onClose }: Props = $props()

  type Tab = 'general' | 'agents' | 'notify' | 'git'
  type Mode = 'edit' | 'new' | 'variant'
  interface Form {
    id: string
    name: string
    extends: string
    cmd: string
    args: string
    env: string
    status: string
    setupHooks: string
    idle: string
    patterns: string
    enabled: boolean
  }

  let tab = $state<Tab>('agents')
  let editing = $state<{ mode: Mode; form: Form } | null>(null)
  let formError = $state<string | null>(null)
  let listError = $state<string | null>(null)
  let confirmFor = $state<string | null>(null)
  let busy = $state(false)
  let scanning = $state(false)
  let dragId = $state<string | null>(null)
  let dropAt = $state<number | null>(null)

  let notify = $state<Settings['notify'] | null>(null)
  /** activity.stall_seconds as typed (validated on save). */
  let stallSeconds = $state('')
  let savedNotify = $state<string>('')
  let notifyError = $state<string | null>(null)
  let notifySaved = $state(false)

  /** git.* as edited in the Git tab. */
  let git = $state<Settings['git'] | null>(null)
  let savedGit = ''
  let gitError = $state<string | null>(null)
  let gitSaved = $state(false)
  const gitDirty = $derived(git != null && JSON.stringify(git) !== savedGit)

  const installedCount = $derived(profiles.filter((p) => p.installed).length)
  /** What ⌘N launches: the first enabled, installed, valid profile. */
  const firstId = $derived(launchable(profiles)[0]?.id)

  /** ui.language as selected (applied right away, then saved). */
  let language = $state<LangSetting>(langSetting())
  let languageError = $state<string | null>(null)

  async function saveLanguage(v: LangSetting) {
    const prev = langSetting()
    language = v
    setLangSetting(v)
    languageError = null
    try {
      const s = await api.saveSettings({ ui: { language: v } })
      setLangSetting(s.ui?.language)
      language = langSetting()
      onSettings?.(s)
    } catch (e) {
      setLangSetting(prev)
      language = prev
      languageError = errorMessage(e)
    }
  }
  const notifyDirty = $derived(notify != null && JSON.stringify([notify, stallSeconds.trim()]) !== savedNotify)

  function sourceLabel(p: Profile): string {
    if (p.extends) return t('settings.source.variant', { id: p.extends })
    switch (p.source) {
      case 'catalog':
        return t('settings.source.catalog')
      case 'override':
        return t('settings.source.override')
      default:
        return t('settings.source.custom')
    }
  }

  function fromProfile(p: Profile): Form {
    return {
      id: p.id,
      name: p.name,
      extends: p.extends ?? '',
      cmd: p.cmd,
      args: joinArgs(p.args),
      env: Object.entries(p.env ?? {})
        .map(([k, v]) => `${k}=${v}`)
        .join('\n'),
      status: p.status || 'heuristic',
      setupHooks: p.setupHooks || 'none',
      idle: p.idleSeconds ? String(p.idleSeconds) : '',
      patterns: (p.attentionPatterns ?? []).join('\n'),
      enabled: p.enabled,
    }
  }

  function lines(s: string): string[] {
    return s
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l !== '')
  }

  function toEntry(f: Form): ProfileEntry {
    const env: Record<string, string> = {}
    for (const l of lines(f.env)) {
      const i = l.indexOf('=')
      if (i <= 0) throw new Error(t('settings.err.env', { line: l }))
      env[l.slice(0, i).trim()] = l.slice(i + 1)
    }
    let idle = 0
    if (f.idle.trim() !== '') {
      idle = Number(f.idle.replace(',', '.'))
      if (!Number.isFinite(idle) || idle <= 0) throw new Error(t('settings.err.idle'))
    }
    return {
      id: f.id.trim(),
      name: f.name.trim() || undefined,
      extends: f.extends || undefined,
      enabled: f.enabled,
      cmd: f.cmd.trim() || undefined,
      args: splitArgs(f.args),
      env,
      status: f.status,
      setupHooks: f.setupHooks,
      idleSeconds: idle || undefined,
      attentionPatterns: lines(f.patterns),
    }
  }

  function uniqueId(base: string): string {
    const taken = new Set(profiles.map((p) => p.id))
    for (let n = 2; ; n++) {
      const id = `${base}-${n}`
      if (!taken.has(id)) return id
    }
  }

  function startEdit(p: Profile) {
    confirmFor = null
    formError = null
    editing = { mode: 'edit', form: fromProfile(p) }
  }

  function startVariant(p: Profile) {
    confirmFor = null
    formError = null
    editing = {
      mode: 'variant',
      form: { ...fromProfile(p), id: uniqueId(p.id), name: t('settings.variantName', { name: p.name }), extends: p.id, enabled: true },
    }
  }

  function startNew() {
    confirmFor = null
    formError = null
    editing = {
      mode: 'new',
      form: {
        id: '',
        name: '',
        extends: '',
        cmd: '',
        args: '',
        env: '',
        status: 'heuristic',
        setupHooks: 'none',
        idle: '',
        patterns: '',
        enabled: true,
      },
    }
  }

  async function run(op: () => Promise<Profile[]>, onError: (msg: string) => void): Promise<boolean> {
    busy = true
    try {
      onProfiles(await op())
      return true
    } catch (e) {
      onError(errorMessage(e))
      return false
    } finally {
      busy = false
    }
  }

  async function submit(ev: SubmitEvent) {
    ev.preventDefault()
    if (!editing) return
    let entry: ProfileEntry
    try {
      entry = toEntry(editing.form)
    } catch (e) {
      formError = errorMessage(e)
      return
    }
    if (editing.mode !== 'edit' && profiles.some((p) => p.id === entry.id)) {
      formError = t('settings.err.exists', { id: entry.id })
      return
    }
    formError = null
    if (await run(() => api.saveProfile(entry), (m) => (formError = m))) editing = null
  }

  async function toggle(p: Profile) {
    listError = null
    let entry: ProfileEntry
    try {
      entry = toEntry({ ...fromProfile(p), enabled: !p.enabled })
    } catch (e) {
      listError = errorMessage(e)
      return
    }
    await run(() => api.saveProfile(entry), (m) => (listError = `${p.id}: ${m}`))
  }

  async function remove(p: Profile) {
    confirmFor = null
    listError = null
    if (editing?.form.id === p.id) editing = null
    await run(() => api.deleteProfile(p.id), (m) => (listError = `${p.id}: ${m}`))
  }

  async function reorder(ids: string[]) {
    listError = null
    await run(() => api.reorderProfiles(ids), (m) => (listError = m))
  }

  function move(i: number, delta: number) {
    const j = i + delta
    if (j < 0 || j >= profiles.length) return
    const ids = profiles.map((p) => p.id)
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    void reorder(ids)
  }

  function onDragStart(ev: DragEvent, id: string) {
    dragId = id
    ev.dataTransfer?.setData('text/plain', id)
    if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(ev: DragEvent, i: number) {
    if (!dragId) return
    ev.preventDefault()
    const el = ev.currentTarget as HTMLElement
    const r = el.getBoundingClientRect()
    dropAt = ev.clientY < r.top + r.height / 2 ? i : i + 1
  }

  function onDrop(ev: DragEvent) {
    ev.preventDefault()
    const from = profiles.findIndex((p) => p.id === dragId)
    const at = dropAt
    dragId = null
    dropAt = null
    if (from < 0 || at == null) return
    const ids = profiles.map((p) => p.id)
    const [id] = ids.splice(from, 1)
    ids.splice(at > from ? at - 1 : at, 0, id)
    if (ids.join() !== profiles.map((p) => p.id).join()) void reorder(ids)
  }

  function onDragEnd() {
    dragId = null
    dropAt = null
  }

  async function rescan() {
    scanning = true
    listError = null
    await run(() => api.rescanProfiles(), (m) => (listError = m))
    scanning = false
  }

  async function loadSettings() {
    try {
      const s = await api.getSettings()
      applySettings(s)
      notifyError = null
    } catch (e) {
      notifyError = errorMessage(e)
    }
  }

  function applySettings(s: Settings) {
    notify = { ...s.notify, notifyOnStall: s.notify.notifyOnStall ?? true }
    stallSeconds = String(s.activity?.stallSeconds ?? 180)
    savedNotify = JSON.stringify([notify, stallSeconds.trim()])
    git = {
      editor: s.git?.editor ?? '',
      worktreeDir: s.git?.worktreeDir ?? '',
      defaultMode: s.git?.defaultMode === 'worktree' ? 'worktree' : 'same',
    }
    savedGit = JSON.stringify(git)
    setLangSetting(s.ui?.language)
    language = langSetting()
  }

  async function saveGit() {
    if (!git) return
    gitSaved = false
    const dir = git.worktreeDir.trim()
    if (dir && !dir.startsWith('/') && !dir.startsWith('~/')) {
      gitError = t('settings.err.worktreeDir')
      return
    }
    try {
      const s = await api.saveSettings({ git: { editor: git.editor.trim(), worktreeDir: dir, defaultMode: git.defaultMode } })
      applySettings(s)
      onSettings?.(s)
      gitError = null
      gitSaved = true
    } catch (e) {
      gitError = errorMessage(e)
    }
  }

  async function saveNotify() {
    if (!notify) return
    notifySaved = false
    const secs = Number(stallSeconds.trim())
    if (stallSeconds.trim() === '' || !Number.isInteger(secs) || secs < 0) {
      notifyError = t('settings.err.stall')
      return
    }
    try {
      const s = await api.saveSettings({ notify: { ...notify }, activity: { stallSeconds: secs } })
      applySettings(s)
      notifyError = null
      notifySaved = true
    } catch (e) {
      notifyError = errorMessage(e)
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    e.preventDefault()
    if (confirmFor) confirmFor = null
    else if (editing) editing = null
    else onClose()
  }

  onMount(() => {
    void loadSettings()
  })
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet editor(f: Form, mode: Mode)}
  <form class="editor" onsubmit={submit}>
    <div class="grid">
      {#if mode !== 'edit'}
        <label>
          <span>{t('settings.f.id')}</span>
          <!-- svelte-ignore a11y_autofocus -->
          <input class="mono" bind:value={f.id} placeholder={t('settings.f.idPlaceholder')} autofocus required spellcheck="false" />
        </label>
      {/if}
      <label>
        <span>{t('settings.f.name')}</span>
        <input bind:value={f.name} placeholder={f.id || t('settings.f.namePlaceholder')} />
      </label>
      {#if f.extends}
        <label>
          <span>{t('settings.f.variantOf')}</span>
          <input class="mono" value={f.extends} disabled />
        </label>
      {/if}
      <label>
        <span>{t('settings.f.cmd')}</span>
        <input class="mono" bind:value={f.cmd} placeholder={t('settings.f.cmdPlaceholder')} spellcheck="false" required={!f.extends} />
      </label>
      <label class="wide">
        <span>{t('settings.f.args')}</span>
        <input class="mono" bind:value={f.args} placeholder={t('settings.f.argsPlaceholder')} spellcheck="false" />
      </label>
      <label class="wide">
        <span>{t('settings.f.env')} <em>{t('settings.f.envHelp')}</em></span>
        <textarea class="mono" rows="2" bind:value={f.env} spellcheck="false"></textarea>
      </label>
      <label>
        <span>{t('settings.f.status')}</span>
        <select bind:value={f.status}>
          <option value="heuristic">{t('settings.f.statusHeuristic')}</option>
          <option value="hooks">{t('settings.f.statusHooks')}</option>
        </select>
      </label>
      <label>
        <span>{t('settings.f.hooks')}</span>
        <select bind:value={f.setupHooks}>
          <option value="none">{t('settings.f.hooksNone')}</option>
          <option value="claude">claude</option>
          <option value="codex">codex</option>
        </select>
      </label>
      <label>
        <span>{t('settings.f.idle')}</span>
        <input bind:value={f.idle} inputmode="decimal" placeholder={t('settings.f.idlePlaceholder')} />
      </label>
      <label class="wide">
        <span>{t('settings.f.patterns')} <em>{t('settings.f.patternsHelp')}</em></span>
        <textarea class="mono" rows="2" bind:value={f.patterns} spellcheck="false" placeholder={'\\(y/n\\)'}></textarea>
      </label>
    </div>
    {#if formError}<div class="error">{formError}</div>{/if}
    <div class="editor-actions">
      <button type="button" class="btn" onclick={() => (editing = null)}>{t('settings.cancel')}</button>
      <button type="submit" class="btn primary" disabled={busy}>
        {mode === 'edit' ? t('settings.save') : t('settings.create')}
      </button>
    </div>
  </form>
{/snippet}

<div class="settings">
  <header class="titlebar drag">
    <span class="title">{t('settings.title')}</span>
    <nav class="tabs">
      <button class:active={tab === 'general'} onclick={() => (tab = 'general')}>{t('settings.tab.general')}</button>
      <button class:active={tab === 'agents'} onclick={() => (tab = 'agents')}>{t('settings.tab.agents')}</button>
      <button class:active={tab === 'notify'} onclick={() => (tab = 'notify')}>{t('settings.tab.notify')}</button>
      <button class:active={tab === 'git'} onclick={() => (tab = 'git')}>{t('settings.tab.git')}</button>
    </nav>
    <button class="close" title={t('settings.close')} onclick={onClose}>×</button>
  </header>

  <div class="body">
    {#if tab === 'general'}
      <section class="pane narrow">
        <label class="field">
          <span>
            <b>{t('settings.language')}</b>
            <em>{t('settings.languageHelp')}</em>
          </span>
          <select
            class="lang"
            value={language}
            onchange={(e) => saveLanguage((e.currentTarget as HTMLSelectElement).value as LangSetting)}
          >
            <option value="auto">{t('settings.languageAuto', { lang: systemLang() === 'es' ? 'Español' : 'English' })}</option>
            <option value="en">English</option>
            <option value="es">Español</option>
          </select>
        </label>
        {#if languageError}<div class="error">{languageError}</div>{/if}
        <p class="hint">{@html th('settings.languageHint')}</p>
      </section>
    {:else if tab === 'agents'}
      <section class="pane">
        <div class="toolbar">
          <p class="hint">
            {@html th('settings.agentsHint', { installed: installedCount, total: profiles.length })}
          </p>
          <button class="btn" onclick={rescan} disabled={scanning}>{scanning ? t('settings.scanning') : t('settings.rescan')}</button>
          <button class="btn" onclick={startNew}>{t('settings.customAgent')}</button>
        </div>
        {#if listError}<div class="error">{listError}</div>{/if}

        {#if editing?.mode === 'new'}
          <div class="card editing">
            <div class="card-title">{t('settings.newAgent')}</div>
            {@render editor(editing.form, 'new')}
          </div>
        {/if}

        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <ul class="list" ondrop={onDrop} ondragover={(e) => dragId && e.preventDefault()}>
          {#each profiles as p, i (p.id)}
            <li
              class="row"
              class:off={!p.enabled || !p.installed}
              class:dragging={dragId === p.id}
              class:drop-before={dropAt === i && dragId !== p.id}
              class:drop-after={dropAt === i + 1 && i === profiles.length - 1}
              ondragover={(e) => onDragOver(e, i)}
            >
              <div class="row-main">
                <span
                  class="handle"
                  title={t('settings.drag')}
                  draggable="true"
                  role="button"
                  tabindex="-1"
                  ondragstart={(e) => onDragStart(e, p.id)}
                  ondragend={onDragEnd}>⋮⋮</span
                >
                <span class="order">
                  <button class="mini" title={t('settings.up')} disabled={i === 0 || busy} onclick={() => move(i, -1)}>▲</button>
                  <button class="mini" title={t('settings.down')} disabled={i === profiles.length - 1 || busy} onclick={() => move(i, 1)}
                    >▼</button
                  >
                </span>
                <div class="who">
                  <div class="name">
                    {p.name}
                    {#if p.id === firstId}<span class="tag first">⌘N</span>{/if}
                    <span class="tag">{sourceLabel(p)}</span>
                  </div>
                  <div class="cmd" title={p.path ?? ''}>
                    <span class="id">{p.id}</span> · {p.cmd}{#if p.args?.length}&nbsp;{joinArgs(p.args)}{/if}
                  </div>
                </div>
                {#if p.error}
                  <span class="badge err" title={p.error}>{t('settings.badgeError')}</span>
                {:else if p.installed}
                  <span class="badge ok" title={p.path ?? ''}>{t('settings.installed')}</span>
                {:else}
                  <span class="badge missing">{t('settings.notFound')}</span>
                {/if}
                <button
                  class="switch"
                  class:on={p.enabled}
                  role="switch"
                  aria-checked={p.enabled}
                  aria-label={p.enabled ? t('settings.disable') : t('settings.enable')}
                  title={p.enabled ? t('settings.enabledTitle') : t('settings.disabledTitle')}
                  disabled={busy}
                  onclick={() => toggle(p)}><span></span></button
                >
                <span class="actions">
                  {#if confirmFor === p.id}
                    <span class="confirm">
                      {p.source === 'override' ? t('settings.confirmReset') : t('settings.confirmDelete')}
                      <button class="btn danger" onclick={() => remove(p)}>{t('settings.yes')}</button>
                      <button class="btn" onclick={() => (confirmFor = null)}>{t('settings.no')}</button>
                    </span>
                  {:else}
                    <button class="link" onclick={() => (editing?.form.id === p.id && editing.mode === 'edit' ? (editing = null) : startEdit(p))}
                      >{t('settings.edit')}</button
                    >
                    <button class="link" onclick={() => startVariant(p)}>{t('settings.duplicate')}</button>
                    {#if p.source === 'custom'}
                      <button class="link danger" onclick={() => (confirmFor = p.id)}>{t('settings.delete')}</button>
                    {:else if p.source === 'override'}
                      <button class="link" onclick={() => (confirmFor = p.id)}>{t('settings.reset')}</button>
                    {/if}
                  {/if}
                </span>
              </div>
              {#if p.error}
                <div class="row-note err">{p.error}</div>
              {:else if !p.installed}
                <div class="row-note">
                  {#if p.install}{t('settings.install')} <code>{p.install}</code>{:else}{@html th('settings.notInPath', { cmd: p.cmd })}{/if}
                  {#if p.homepage}
                    <button class="link" onclick={() => BrowserOpenURL(p.homepage!)}>{t('settings.web')}</button>
                  {/if}
                </div>
              {/if}
              {#if editing && editing.mode !== 'new' && (editing.mode === 'edit' ? editing.form.id === p.id : editing.form.extends === p.id)}
                <div class="card editing">
                  {#if editing.mode === 'variant'}<div class="card-title">{t('settings.newVariant', { name: p.name })}</div>{/if}
                  {@render editor(editing.form, editing.mode)}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
        <p class="hint foot">
          {@html th('settings.agentsFoot')}
        </p>
      </section>
    {:else if tab === 'notify'}
      <section class="pane narrow">
        {#if notify}
          <label class="check">
            <input type="checkbox" bind:checked={notify.enabled} />
            <span>
              <b>{t('settings.n.enabled')}</b>
              <em>{t('settings.n.enabledHelp')}</em>
            </span>
          </label>
          <label class="check" class:dim={!notify.enabled}>
            <input type="checkbox" bind:checked={notify.sound} disabled={!notify.enabled} />
            <span>
              <b>{t('settings.n.sound')}</b>
              <em>{t('settings.n.soundHelp')}</em>
            </span>
          </label>
          <label class="check" class:dim={!notify.enabled}>
            <input type="checkbox" bind:checked={notify.notifyOnDone} disabled={!notify.enabled} />
            <span>
              <b>{t('settings.n.done')}</b>
              <em>{t('settings.n.doneHelp')}</em>
            </span>
          </label>
          <label class="check" class:dim={!notify.enabled}>
            <input type="checkbox" bind:checked={notify.notifyOnStall} disabled={!notify.enabled} />
            <span>
              <b>{t('settings.n.stall')}</b>
              <em>{t('settings.n.stallHelp')}</em>
            </span>
          </label>
          <label class="field">
            <span>
              <b>{t('settings.n.stallSeconds')}</b>
              <em>{t('settings.n.stallSecondsHelp')}</em>
            </span>
            <input class="num" bind:value={stallSeconds} inputmode="numeric" placeholder="180" />
          </label>
          <div class="editor-actions left">
            <button class="btn primary" disabled={!notifyDirty} onclick={saveNotify}>{t('settings.save')}</button>
            {#if notifySaved && !notifyDirty}<span class="saved">{t('settings.saved')}</span>{/if}
          </div>
          <p class="hint">{@html th('settings.n.hint')}</p>
        {/if}
        {#if notifyError}
          <div class="error">{notifyError}</div>
          <button class="btn" onclick={loadSettings}>{t('settings.retry')}</button>
        {/if}
      </section>
    {:else}
      <section class="pane narrow">
        {#if git}
          <div class="check">
            <span>
              <b>{t('settings.g.mode')}</b>
              <em>{@html th('settings.g.modeHelp')}</em>
              <span class="radios">
                <label><input type="radio" bind:group={git.defaultMode} value="same" /> {t('settings.g.same')}</label>
                <label><input type="radio" bind:group={git.defaultMode} value="worktree" /> {t('settings.g.worktree')}</label>
              </span>
            </span>
          </div>
          <label class="field">
            <span>
              <b>{t('settings.g.editor')}</b>
              <em>{@html th('settings.g.editorHelp')}</em>
            </span>
            <input class="text" bind:value={git.editor} placeholder="code" spellcheck="false" />
          </label>
          <label class="field">
            <span>
              <b>{t('settings.g.dir')}</b>
              <em>{@html th('settings.g.dirHelp')}</em>
            </span>
            <input class="text" bind:value={git.worktreeDir} placeholder="~/.agenterm/worktrees" spellcheck="false" />
          </label>
          <div class="editor-actions left">
            <button class="btn primary" disabled={!gitDirty} onclick={saveGit}>{t('settings.save')}</button>
            {#if gitSaved && !gitDirty}<span class="saved">{t('settings.saved')}</span>{/if}
          </div>
          <p class="hint">{@html th('settings.g.hint')}</p>
        {/if}
        {#if gitError}
          <div class="error">{gitError}</div>
        {/if}
        {#if notifyError && !git}
          <div class="error">{notifyError}</div>
          <button class="btn" onclick={loadSettings}>{t('settings.retry')}</button>
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .settings {
    position: absolute;
    inset: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }
  .titlebar {
    height: 38px;
    flex: none;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--border);
  }
  .title {
    font-weight: 600;
  }
  .tabs {
    display: flex;
    gap: 2px;
    flex: 1;
  }
  .tabs button {
    padding: 3px 10px;
    border-radius: 5px;
    color: var(--fg-dim);
  }
  .tabs button:hover {
    background: var(--hover);
  }
  .tabs button.active {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .close {
    width: 24px;
    height: 24px;
    border-radius: 5px;
    color: var(--muted);
    font-size: 17px;
    line-height: 1;
  }
  .close:hover {
    background: var(--hover-strong);
    color: var(--fg);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }
  .pane {
    max-width: 860px;
    margin: 0 auto;
    padding: 16px 20px 28px;
  }
  .pane.narrow {
    max-width: 560px;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
  }
  .hint {
    flex: 1;
    margin: 0;
    color: var(--muted);
    font-size: 12px;
  }
  .hint.foot {
    margin-top: 12px;
  }
  /* :global: also styles the markup of {@html th(...)} strings. */
  .settings :global(kbd),
  .settings :global(code) {
    font-family: var(--mono);
    font-size: 11px;
  }
  .settings :global(kbd) {
    padding: 0 4px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
  }
  .settings :global(code) {
    color: var(--fg-dim);
    user-select: text;
  }
  .btn {
    flex: none;
    padding: 4px 11px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
    color: var(--fg);
    background: var(--hover);
    white-space: nowrap;
  }
  .btn:hover:not(:disabled) {
    background: var(--hover-strong);
  }
  .btn:disabled {
    opacity: 0.45;
  }
  .btn.primary {
    background: var(--accent-bg);
    border-color: color-mix(in srgb, var(--blue) 40%, transparent);
  }
  .btn.primary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--blue) 30%, var(--accent-bg));
  }
  .btn.danger {
    color: var(--red);
    border-color: color-mix(in srgb, var(--red) 40%, transparent);
  }
  .link {
    color: var(--blue);
    font-size: 12px;
  }
  .link:hover {
    text-decoration: underline;
  }
  .link.danger {
    color: var(--red);
  }
  .error {
    margin: 8px 0;
    padding: 6px 10px;
    border-radius: 6px;
    background: #3b1d22;
    color: #f2b8be;
    border: 1px solid #5a2a31;
    font-size: 12px;
    user-select: text;
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
  }
  .row {
    position: relative;
    border-top: 1px solid var(--border);
    background: var(--sidebar-bg);
  }
  .row:first-child {
    border-top: none;
  }
  .row.dragging {
    opacity: 0.4;
  }
  .row.drop-before::before,
  .row.drop-after::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    height: 2px;
    background: var(--blue);
  }
  .row.drop-before::before {
    top: -1px;
  }
  .row.drop-after::after {
    bottom: -1px;
  }
  .row-main {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 48px;
    padding: 6px 12px 6px 6px;
  }
  .row.off .who {
    opacity: 0.55;
  }
  .handle {
    color: var(--muted);
    font-size: 11px;
    letter-spacing: -2px;
    cursor: grab;
    padding: 4px 2px;
  }
  .order {
    display: flex;
    flex-direction: column;
  }
  .mini {
    width: 16px;
    height: 13px;
    font-size: 8px;
    line-height: 1;
    color: var(--muted);
    border-radius: 3px;
  }
  .mini:hover:not(:disabled) {
    color: var(--fg);
    background: var(--hover-strong);
  }
  .mini:disabled {
    opacity: 0.25;
  }
  .who {
    flex: 1;
    min-width: 0;
  }
  .name {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 600;
  }
  .tag {
    padding: 0 6px;
    border-radius: 8px;
    font-size: 10.5px;
    font-weight: 400;
    color: var(--muted);
    border: 1px solid var(--border-strong);
  }
  .tag.first {
    color: var(--blue);
    border-color: color-mix(in srgb, var(--blue) 40%, transparent);
  }
  .cmd {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11px;
  }
  .id {
    color: var(--fg-dim);
  }
  .badge {
    flex: none;
    padding: 1px 8px;
    border-radius: 10px;
    font-size: 11px;
  }
  .badge.ok {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--green) 30%, transparent);
  }
  .badge.missing {
    color: var(--muted);
    border: 1px solid var(--border-strong);
  }
  .badge.err {
    color: var(--red);
    background: color-mix(in srgb, var(--red) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--red) 30%, transparent);
  }
  .switch {
    flex: none;
    position: relative;
    width: 30px;
    height: 17px;
    border-radius: 9px;
    background: var(--border-strong);
    transition: background 0.12s;
  }
  .switch span {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 13px;
    height: 13px;
    border-radius: 50%;
    background: var(--fg-dim);
    transition: transform 0.12s;
  }
  .switch.on {
    background: color-mix(in srgb, var(--blue) 70%, var(--bg));
  }
  .switch.on span {
    transform: translateX(13px);
    background: #fff;
  }
  .switch:disabled {
    opacity: 0.6;
  }
  .actions {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 230px;
    justify-content: flex-end;
  }
  .confirm {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--fg-dim);
  }
  .row-note {
    padding: 0 12px 8px 60px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .row-note.err {
    color: var(--red);
  }
  .card {
    margin: 0 12px 12px 60px;
    padding: 12px;
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    background: var(--menu-bg);
  }
  .pane > .card {
    margin: 0 0 12px;
  }
  .card-title {
    margin-bottom: 8px;
    font-weight: 600;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 10px 12px;
  }
  .grid .wide {
    grid-column: 1 / -1;
  }
  .grid label {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 11.5px;
    color: var(--fg-dim);
  }
  em {
    font-style: normal;
    color: var(--muted);
  }
  input:not([type='checkbox']),
  select,
  textarea {
    font: inherit;
    font-size: 12.5px;
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    padding: 5px 7px;
    outline: none;
    resize: vertical;
  }
  input:focus,
  select:focus,
  textarea:focus {
    border-color: color-mix(in srgb, var(--blue) 60%, transparent);
  }
  input:disabled {
    color: var(--muted);
  }
  .mono {
    font-family: var(--mono);
    font-size: 12px;
  }
  .editor-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    margin-top: 12px;
  }
  .editor-actions.left {
    justify-content: flex-start;
  }
  .saved {
    color: var(--green);
    font-size: 12px;
  }
  .check {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 0;
    border-bottom: 1px solid var(--border);
  }
  .check span {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .check em {
    font-size: 12px;
  }
  .check.dim {
    opacity: 0.5;
  }
  .check input {
    margin-top: 3px;
    accent-color: var(--blue);
  }
  .field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 0 10px 23px;
    border-bottom: 1px solid var(--border);
  }
  .field span {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .field em {
    font-size: 12px;
  }
  .field .num {
    width: 80px;
    flex: none;
    text-align: right;
  }
  .field .lang {
    flex: none;
    min-width: 200px;
  }
  .field .text {
    width: 220px;
    flex: none;
    font-family: var(--mono);
    font-size: 12px;
  }
  .radios {
    display: flex !important;
    flex-direction: row !important;
    gap: 16px !important;
    margin-top: 6px;
  }
  .radios label {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .radios input {
    accent-color: var(--blue);
  }
</style>
