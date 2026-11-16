<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { EventsOn, WindowSetTitle } from '../wailsjs/runtime/runtime'
  import Changes from './lib/Changes.svelte'
  import Grid from './lib/Grid.svelte'
  import ProgressBar from './lib/ProgressBar.svelte'
  import Settings from './lib/Settings.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import { activityText, mergeLive, progressOf, timerInfo } from './lib/activity'
  import { setLangSetting, setSystemLang, t, th } from './lib/i18n.svelte'
  import { TerminalManager } from './lib/terminals'
  import {
    api,
    attentionLabel,
    errorMessage,
    launchable,
    needsAttention,
    rememberedMode,
    type DaemonEvent,
    type Profile,
    type Project,
    type Session,
    type SpawnMode,
    type SpawnOptions,
    type WorktreeInfo,
  } from './lib/api'

  let projects = $state<Project[]>([])
  let sessions = $state<Session[]>([])
  let profiles = $state<Profile[]>([])
  let selectedId = $state<string | null>(null)
  let currentProjectId = $state<string | null>(null)
  let error = $state<string | null>(null)
  let daemonDown = $state<string | null>(null)
  let loaded = $state(false)
  let settingsOpen = $state(false)
  let gridOpen = $state(false)
  /** Project the grid shows (null: the current one). */
  let gridProjectId = $state<string | null>(null)
  /** Ticks every second for the elapsed timers. */
  let now = $state(Date.now())
  /** agenterm worktrees per project (for the orphan lists). */
  let worktrees = $state<Record<string, WorktreeInfo[]>>({})
  /** Which tab each session shows: terminal (default) or its changes. */
  let views = $state<Record<string, 'terminal' | 'changes'>>({})
  /** An orphaned worktree's changes shown over the main area. */
  let orphanView = $state<{ id: string; remove: boolean } | null>(null)
  /** settings git.default_mode (what ⌘N / the + menu start with). */
  let defaultMode: SpawnMode = 'same'

  let termHost: HTMLDivElement
  let errorTimer: number | undefined

  const selected = $derived(sessions.find((s) => s.id === selectedId) ?? null)
  const currentProject = $derived(projects.find((p) => p.id === currentProjectId) ?? null)
  const attentionCount = $derived(sessions.filter(needsAttention).length)
  /** What the spawn menus offer, in order (the first one is ⌘N's). */
  const spawnable = $derived(launchable(profiles))
  const selectedTimer = $derived(selected ? timerInfo(selected, now, true) : null)
  const selectedProgress = $derived(selected && selected.state !== 'exited' ? progressOf(selected) : undefined)
  /** The selected session has a git folder: offer the Cambios tab. */
  const hasChanges = $derived(!!selected && (selected.mode === 'worktree' || !!selected.git))
  const view = $derived(selected && hasChanges ? (views[selected.id] ?? 'terminal') : 'terminal')

  function setView(v: 'terminal' | 'changes') {
    if (!selected) return
    views[selected.id] = v
    if (v === 'terminal') terms.focus()
    else if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
  }

  async function refreshWorktrees(projectId: string) {
    try {
      const list = await api.listWorktrees(projectId)
      worktrees = { ...worktrees, [projectId]: list }
    } catch {
      /* older daemon or not a repo: no list */
    }
  }

  function refreshAllWorktrees() {
    for (const p of projects) void refreshWorktrees(p.id)
  }

  function openOrphan(id: string, remove = false) {
    gridOpen = false
    orphanView = { id, remove }
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
  }

  function closeOrphan() {
    orphanView = null
    terms.focus()
  }

  $effect(() => {
    WindowSetTitle(attentionCount ? `agenterm (${attentionCount})` : 'agenterm')
  })

  /** Acks the selected session if the user can actually see it. */
  function ackIfVisible(id: string | null) {
    if (!id || !document.hasFocus()) return
    const s = sessions.find((x) => x.id === id)
    if (s && needsAttention(s)) void api.ack(id).catch(() => {})
  }

  function isAppShortcut(e: KeyboardEvent): boolean {
    if (!e.metaKey || e.ctrlKey || e.altKey) return false
    if (/^Digit[0-9]$/.test(e.code)) return true
    if (e.key === ',' && !e.shiftKey) return true
    return e.key.toLowerCase() === 'n' && !e.shiftKey
  }

  const terms = new TerminalManager(isAppShortcut, showError)

  function showError(msg: string) {
    error = msg
    clearTimeout(errorTimer)
    errorTimer = window.setTimeout(() => (error = null), 6000)
  }

  async function guard<T>(p: Promise<T>): Promise<T | undefined> {
    try {
      return await p
    } catch (e) {
      showError(errorMessage(e))
      return undefined
    }
  }

  async function refreshSessions() {
    const list = await guard(api.listSessions())
    if (!list) return
    sessions = list
    terms.retain(new Set(list.map((s) => s.id)))
    if (selectedId && !list.some((s) => s.id === selectedId)) select(fallbackSelection(), true)
    // Nothing selected and a session appeared in the current project (e.g.
    // spawned from the CLI): show it rather than "No sessions" next to it.
    else if (loaded && !selectedId && list.some((s) => s.projectId === currentProjectId)) select(fallbackSelection(), true)
  }

  async function refreshAll() {
    const [ps, profs] = await Promise.all([guard(api.listProjects()), guard(api.listProfiles())])
    if (ps) projects = ps
    if (profs) profiles = profs
    await refreshSessions()
    if (!currentProjectId || !projects.some((p) => p.id === currentProjectId)) {
      currentProjectId = projects[0]?.id ?? null
    }
    if (!selectedId) select(fallbackSelection(), true)
    loaded = true
    refreshAllWorktrees()
    void api
      .getSettings()
      .then((st) => {
        defaultMode = st.git?.defaultMode === 'worktree' ? 'worktree' : 'same'
        setLangSetting(st.ui?.language)
      })
      .catch(() => {})
  }

  function fallbackSelection(): string | null {
    const inProject = sessions.filter((s) => s.projectId === currentProjectId)
    return (inProject.find((s) => s.state !== 'exited') ?? inProject[0] ?? sessions[0])?.id ?? null
  }

  function select(id: string | null, keepGrid = false) {
    if (!keepGrid) gridOpen = false
    selectedId = id
    const s = id ? sessions.find((x) => x.id === id) : undefined
    if (s) currentProjectId = s.projectId
    terms.show(id)
    void api.setVisibleSession(id ?? '').catch(() => {})
    ackIfVisible(id)
  }

  function openGrid(projectId?: string) {
    gridProjectId = projectId ?? null
    if (projectId) currentProjectId = projectId
    gridOpen = true
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
  }

  function closeGrid() {
    gridOpen = false
    terms.focus()
  }

  function openFromGrid(id: string) {
    gridOpen = false
    select(id)
  }

  function selectProject(id: string) {
    currentProjectId = id
    if (gridOpen) gridProjectId = id // the grid follows the project
    const first = sessions.find((s) => s.projectId === id)
    if (first && selected?.projectId !== id) select(first.id, gridOpen)
  }

  function openSettings() {
    settingsOpen = true
    // Keep keystrokes out of the (now covered) terminal.
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
    // Detection may be stale (an agent installed meanwhile): refresh.
    void guard(api.listProfiles()).then((ps) => ps && (profiles = ps))
  }

  function closeSettings() {
    settingsOpen = false
    terms.focus()
  }

  async function spawn(projectId: string, profileId: string, opts?: Partial<SpawnOptions>) {
    const { cols, rows } = terms.proposeSize()
    const info = await guard(api.spawn(projectId, profileId, cols, rows, opts))
    if (!info) return
    if (!sessions.some((s) => s.id === info.id)) sessions = [...sessions, info]
    if (opts?.worktreeId) orphanView = null
    select(info.id)
  }

  async function kill(id: string) {
    const info = await guard(api.kill(id))
    if (!info) return
    // A second kill of an exited session removes it; the event updates the list.
    sessions = sessions.map((s) => (s.id === id ? { ...s, ...info } : s))
  }

  async function addProject() {
    const p = await guard(api.addProject())
    if (!p) return
    if (!projects.some((x) => x.id === p.id)) projects = [...projects, p]
    currentProjectId = p.id
  }

  async function removeProject(id: string) {
    try {
      await api.removeProject(id)
    } catch (e) {
      showError(errorMessage(e))
      return
    }
    projects = projects.filter((p) => p.id !== id)
    if (currentProjectId === id) currentProjectId = projects[0]?.id ?? null
  }

  function onDaemonEvent(ev: DaemonEvent) {
    switch (ev.type) {
      case 'session.state':
      case 'session.attention':
        // Attention fields are omitted when cleared, so copy them as-is.
        sessions = sessions.map((s) =>
          s.id === ev.sessionId
            ? mergeLive(
                {
                  ...s,
                  state: ev.state ?? s.state,
                  exitCode: ev.exitCode ?? s.exitCode,
                  attentionKind: ev.attentionKind,
                  attentionMessage: ev.attentionMessage,
                  attentionAt: ev.attentionAt,
                  unread: ev.unread,
                },
                ev,
              )
            : s,
        )
        if (ev.raised && ev.sessionId === selectedId) ackIfVisible(selectedId)
        break
      case 'session.activity':
        sessions = sessions.map((s) => (s.id === ev.sessionId ? mergeLive(s, ev) : s))
        break
      case 'session.git':
        sessions = sessions.map((s) => (s.id === ev.sessionId ? { ...s, git: ev.git as Session['git'] } : s))
        break
      case 'worktree.changed':
        if (ev.projectId) void refreshWorktrees(ev.projectId)
        break
      case 'session.spawned':
        void refreshSessions()
        break
      case 'session.removed':
        sessions = sessions.filter((s) => s.id !== ev.sessionId)
        terms.dispose(ev.sessionId)
        if (selectedId === ev.sessionId) select(fallbackSelection(), true)
        break
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && orphanView && !settingsOpen) {
      e.preventDefault()
      closeOrphan()
      return
    }
    if (!isAppShortcut(e)) return
    e.preventDefault()
    if (e.key === ',') {
      openSettings()
      return
    }
    if (settingsOpen) return
    if (e.code === 'Digit0') {
      if (gridOpen) closeGrid()
      else openGrid()
      return
    }
    if (e.code.startsWith('Digit')) {
      const n = Number(e.code.slice(5)) - 1
      const list = sessions.filter((s) => s.projectId === currentProjectId)
      if (list[n]) select(list[n].id)
      return
    }
    // ⌘N: new session in the current project with the first profile.
    const pid = currentProjectId ?? projects[0]?.id
    const prof = spawnable[0]
    if (pid && prof) void spawnDefault(pid, prof.id)
  }

  /** ⌘N: the remembered (or default) mode, but like the + menu, the same
   * folder when the project cannot use worktrees (not a git repo…). */
  async function spawnDefault(projectId: string, profileId: string) {
    let mode = rememberedMode(projectId, defaultMode)
    if (mode === 'worktree') {
      const g = await api.projectGit(projectId).catch(() => null)
      if (!g?.isRepo || g.error) mode = 'same'
    }
    await spawn(projectId, profileId, { mode })
  }

  function focusFromNotification(id: string) {
    if (sessions.some((s) => s.id === id)) select(id)
    else void refreshSessions().then(() => sessions.some((s) => s.id === id) && select(id))
  }

  const offs: Array<() => void> = []
  onMount(() => {
    terms.mount(termHost)
    offs.push(
      EventsOn('daemon:event', onDaemonEvent),
      EventsOn('daemon:resync', () => {
        daemonDown = null
        void refreshAll()
      }),
      EventsOn('daemon:down', (msg: string) => (daemonDown = msg)),
      // App menu "Preferences…" (⌘,).
      EventsOn('app:open-settings', openSettings),
      // A notification was clicked.
      EventsOn('app:focus-session', focusFromNotification),
    )
    // What ui.language "auto" means on this Mac.
    void api
      .systemLanguage()
      .then(setSystemLang)
      .catch(() => {})
    // A click on a daemon notification may have launched the app.
    void refreshAll().then(() =>
      guard(api.takePendingFocus()).then((id) => id && focusFromNotification(id)),
    )
    const onFocus = () => {
      void guard(api.listProjects()).then((ps) => ps && (projects = ps))
      ackIfVisible(selectedId)
    }
    window.addEventListener('focus', onFocus)
    offs.push(() => window.removeEventListener('focus', onFocus))
    const tick = window.setInterval(() => (now = Date.now()), 1000)
    offs.push(() => clearInterval(tick))
  })
  onDestroy(() => {
    offs.forEach((off) => off())
    terms.unmount()
  })
</script>

<svelte:window onkeydown={onKeydown} />

<div class="layout">
  <Sidebar
    {projects}
    {sessions}
    profiles={spawnable}
    {selectedId}
    {currentProjectId}
    onSelect={select}
    onSelectProject={selectProject}
    onSpawn={spawn}
    onKill={kill}
    onAddProject={addProject}
    onRemoveProject={removeProject}
    onOpenSettings={openSettings}
    onOpenGrid={openGrid}
    {now}
    {worktrees}
    onOpenWorktree={openOrphan}
  />

  <main class="main">
    <header class="titlebar drag">
      {#if selected}
        <span class="title">{selected.profileId}</span>
        {#if selected.mode === 'worktree' && selected.branch}
          <span class="branch" title={selected.worktreePath}>⎇ {selected.branch}</span>
        {/if}
        {#if selected.attentionKind && selected.state !== 'exited'}
          <span class="attn {selected.attentionKind}" title={selected.attentionMessage ?? ''}>
            {attentionLabel(selected.attentionKind)}{#if selected.attentionMessage}: {selected.attentionMessage}{/if}
          </span>
        {/if}
        {#if selected.state !== 'exited' && selected.activity}
          {@const act = activityText(selected)}
          <span class="activity" title={act}>{act}</span>
        {/if}
        {#if selectedTimer?.text}
          <span class="timer" class:stalled={selectedTimer.stalled}>{selectedTimer.text}</span>
        {/if}
        <span class="subtitle">{selected.cwd}</span>
      {:else if currentProject}
        <span class="title">{currentProject.name}</span>
        <span class="subtitle">{currentProject.path}</span>
      {/if}
    </header>

    {#if selected && hasChanges}
      <nav class="view-tabs">
        <button class:on={view === 'terminal'} onclick={() => setView('terminal')}>{t('view.terminal')}</button>
        <button class:on={view === 'changes'} onclick={() => setView('changes')}
          >{t('view.changes')}{selected.git ? ` (${selected.git.files})` : ''}</button
        >
      </nav>
    {/if}

    <div class="term-area">
      {#if selected && view === 'changes'}
        <Changes
          sessionId={selected.id}
          live={selected.state !== 'exited'}
          gitStamp={selected.git ? `${selected.git.files}|${selected.git.uncommitted}|${selected.git.ahead}` : ''}
          onError={showError}
          onRemoved={() => {
            setView('terminal')
            void refreshSessions()
          }}
        />
      {/if}
      {#if selectedProgress}<div class="pane-progress"><ProgressBar progress={selectedProgress} thick /></div>{/if}
      <div class="term-host" bind:this={termHost}></div>

      {#if loaded && !selected}
        <div class="empty">
          {#if projects.length === 0}
            <h2>{t('empty.noProjects')}</h2>
            <p>{t('empty.addFolder')}</p>
            <button class="primary" onclick={addProject}>{t('empty.addProject')}</button>
          {:else}
            <h2>{t('empty.noSessions')}</h2>
            <p>
              {@html spawnable[0]
                ? th('empty.launchHintKey', { name: spawnable[0].name || spawnable[0].id })
                : th('empty.launchHint')}
            </p>
            {#if currentProjectId && spawnable.length}
              <div class="quick">
                {#each spawnable as prof (prof.id)}
                  <button onclick={() => spawn(currentProjectId!, prof.id)}>{prof.name || prof.id}</button>
                {/each}
              </div>
            {/if}
            <button class="linkish" onclick={openSettings}>{t('empty.chooseAgents')} <kbd>⌘,</kbd></button>
          {/if}
        </div>
      {/if}
    </div>

    {#if gridOpen && !settingsOpen}
      <Grid
        {projects}
        {sessions}
        projectId={gridProjectId ?? currentProjectId}
        {selectedId}
        {now}
        onOpen={openFromGrid}
        onClose={closeGrid}
      />
    {/if}

    {#if orphanView && !settingsOpen}
      <div class="orphan-view">
        {#key orphanView.id}
          <Changes
            worktreeId={orphanView.id}
            startRemove={orphanView.remove}
            onError={showError}
            onClose={closeOrphan}
            onRemoved={() => {
              const pid = Object.keys(worktrees).find((k) => worktrees[k].some((w) => w.id === orphanView?.id))
              closeOrphan()
              if (pid) void refreshWorktrees(pid)
            }}
          />
        {/key}
      </div>
    {/if}

    {#if settingsOpen}
      <Settings
        {profiles}
        onProfiles={(ps) => (profiles = ps)}
        onSettings={(st) => (defaultMode = st.git?.defaultMode === 'worktree' ? 'worktree' : 'same')}
        onClose={closeSettings}
      />
    {/if}

    {#if daemonDown}
      <div class="banner">{t('banner.reconnecting')} <span>{daemonDown}</span></div>
    {/if}
    {#if error}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="toast" onclick={() => (error = null)}>{error}</div>
    {/if}
  </main>
</div>

<style>
  .layout {
    display: flex;
    height: 100vh;
    width: 100vw;
    overflow: hidden;
  }
  .main {
    position: relative;
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }
  .titlebar {
    height: 38px;
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
    overflow: hidden;
  }
  .title {
    font-weight: 600;
    color: var(--fg);
  }
  .attn {
    flex: none;
    max-width: 45%;
    overflow: hidden;
    text-overflow: ellipsis;
    padding: 1px 8px;
    border-radius: 10px;
    font-size: 11px;
    color: var(--amber);
    background: color-mix(in srgb, var(--amber) 12%, transparent);
    border: 1px solid color-mix(in srgb, var(--amber) 35%, transparent);
  }
  .attn.done {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 10%, transparent);
    border-color: color-mix(in srgb, var(--green) 30%, transparent);
  }
  .attn.error {
    color: var(--red);
    background: color-mix(in srgb, var(--red) 10%, transparent);
    border-color: color-mix(in srgb, var(--red) 30%, transparent);
  }
  .activity {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 40%;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--fg-dim);
    font-size: 12px;
  }
  .timer {
    flex: none;
    color: var(--muted);
    font-size: 11.5px;
    font-variant-numeric: tabular-nums;
  }
  .timer.stalled {
    color: #e08a3c;
    font-weight: 600;
  }
  .pane-progress {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    z-index: 2;
  }
  .branch {
    flex: none;
    font: 11.5px var(--mono);
    color: var(--blue);
  }
  .view-tabs {
    flex: none;
    display: flex;
    gap: 2px;
    padding: 0 10px;
    border-bottom: 1px solid var(--border);
  }
  .view-tabs button {
    padding: 5px 10px 4px;
    font-size: 12px;
    color: var(--muted);
    border-bottom: 2px solid transparent;
  }
  .view-tabs button:hover {
    color: var(--fg);
  }
  .view-tabs button.on {
    color: var(--fg);
    border-bottom-color: var(--blue);
  }
  .orphan-view {
    position: absolute;
    inset: 38px 0 0 0;
    z-index: 5;
    background: var(--bg);
  }
  .subtitle {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .term-area {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .term-host {
    position: absolute;
    inset: 4px 0 0 8px;
  }
  .term-host :global(.term-instance) {
    position: absolute;
    inset: 0;
  }
  .empty {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    background: var(--bg);
    color: var(--fg-dim);
    text-align: center;
  }
  .empty h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--fg);
  }
  .empty p {
    margin: 0 0 10px;
    max-width: 340px;
  }
  /* :global: also styles the markup of {@html th(...)} strings. */
  .empty :global(kbd) {
    font-family: var(--mono);
    font-size: 11px;
    padding: 1px 5px;
    border: 1px solid var(--border-strong);
    border-radius: 4px;
  }
  .primary,
  .quick button {
    padding: 6px 14px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
    color: var(--fg);
    background: var(--hover);
  }
  .primary:hover,
  .quick button:hover {
    background: var(--accent-bg);
  }
  .linkish {
    margin-top: 14px;
    color: var(--muted);
    font-size: 12px;
  }
  .linkish:hover {
    color: var(--fg);
  }
  .quick {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 6px;
  }
  .banner {
    position: absolute;
    top: 46px;
    left: 50%;
    transform: translateX(-50%);
    padding: 6px 12px;
    z-index: 30; /* above Changes, the grid, the orphan view and Settings */
    border-radius: 6px;
    background: #3a2f14;
    color: var(--amber);
    font-size: 12px;
  }
  .banner span {
    color: var(--muted);
    margin-left: 6px;
  }
  .toast {
    position: absolute;
    right: 14px;
    bottom: 14px;
    z-index: 30;
    max-width: 420px;
    padding: 8px 12px;
    border-radius: 6px;
    background: #3b1d22;
    color: #f2b8be;
    border: 1px solid #5a2a31;
    font-size: 12px;
    cursor: default;
  }
</style>
