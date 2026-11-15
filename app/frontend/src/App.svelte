<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import Sidebar from './lib/Sidebar.svelte'
  import { TerminalManager } from './lib/terminals'
  import { api, errorMessage, type DaemonEvent, type Profile, type Project, type Session } from './lib/api'

  let projects = $state<Project[]>([])
  let sessions = $state<Session[]>([])
  let profiles = $state<Profile[]>([])
  let selectedId = $state<string | null>(null)
  let currentProjectId = $state<string | null>(null)
  let error = $state<string | null>(null)
  let daemonDown = $state<string | null>(null)
  let loaded = $state(false)

  let termHost: HTMLDivElement
  let errorTimer: number | undefined

  const selected = $derived(sessions.find((s) => s.id === selectedId) ?? null)
  const currentProject = $derived(projects.find((p) => p.id === currentProjectId) ?? null)

  function isAppShortcut(e: KeyboardEvent): boolean {
    if (!e.metaKey || e.ctrlKey || e.altKey) return false
    if (/^Digit[1-9]$/.test(e.code)) return true
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
    if (selectedId && !list.some((s) => s.id === selectedId)) select(fallbackSelection())
  }

  async function refreshAll() {
    const [ps, profs] = await Promise.all([guard(api.listProjects()), guard(api.listProfiles())])
    if (ps) projects = ps
    if (profs) profiles = profs
    await refreshSessions()
    if (!currentProjectId || !projects.some((p) => p.id === currentProjectId)) {
      currentProjectId = projects[0]?.id ?? null
    }
    if (!selectedId) select(fallbackSelection())
    loaded = true
  }

  function fallbackSelection(): string | null {
    const inProject = sessions.filter((s) => s.projectId === currentProjectId)
    return (inProject.find((s) => s.state !== 'exited') ?? inProject[0] ?? sessions[0])?.id ?? null
  }

  function select(id: string | null) {
    selectedId = id
    const s = id ? sessions.find((x) => x.id === id) : undefined
    if (s) currentProjectId = s.projectId
    terms.show(id)
  }

  function selectProject(id: string) {
    currentProjectId = id
    const first = sessions.find((s) => s.projectId === id)
    if (first && selected?.projectId !== id) select(first.id)
  }

  async function spawn(projectId: string, profileId: string) {
    const { cols, rows } = terms.proposeSize()
    const info = await guard(api.spawn(projectId, profileId, cols, rows))
    if (!info) return
    if (!sessions.some((s) => s.id === info.id)) sessions = [...sessions, info]
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
        sessions = sessions.map((s) =>
          s.id === ev.sessionId ? { ...s, state: ev.state ?? s.state, exitCode: ev.exitCode ?? s.exitCode } : s,
        )
        break
      case 'session.spawned':
        void refreshSessions()
        break
      case 'session.removed':
        sessions = sessions.filter((s) => s.id !== ev.sessionId)
        terms.dispose(ev.sessionId)
        if (selectedId === ev.sessionId) select(fallbackSelection())
        break
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (!isAppShortcut(e)) return
    e.preventDefault()
    if (e.code.startsWith('Digit')) {
      const n = Number(e.code.slice(5)) - 1
      const list = sessions.filter((s) => s.projectId === currentProjectId)
      if (list[n]) select(list[n].id)
      return
    }
    // ⌘N: new session in the current project with the first profile.
    const pid = currentProjectId ?? projects[0]?.id
    const prof = profiles[0]
    if (pid && prof) void spawn(pid, prof.id)
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
    )
    void refreshAll()
    const onFocus = () => {
      void guard(api.listProjects()).then((ps) => ps && (projects = ps))
    }
    window.addEventListener('focus', onFocus)
    offs.push(() => window.removeEventListener('focus', onFocus))
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
    {profiles}
    {selectedId}
    {currentProjectId}
    onSelect={select}
    onSelectProject={selectProject}
    onSpawn={spawn}
    onKill={kill}
    onAddProject={addProject}
    onRemoveProject={removeProject}
  />

  <main class="main">
    <header class="titlebar drag">
      {#if selected}
        <span class="title">{selected.profileId}</span>
        <span class="subtitle">{selected.cwd}</span>
      {:else if currentProject}
        <span class="title">{currentProject.name}</span>
        <span class="subtitle">{currentProject.path}</span>
      {/if}
    </header>

    <div class="term-area">
      <div class="term-host" bind:this={termHost}></div>

      {#if loaded && !selected}
        <div class="empty">
          {#if projects.length === 0}
            <h2>Sin proyectos</h2>
            <p>Añade una carpeta para empezar a lanzar agentes en ella.</p>
            <button class="primary" onclick={addProject}>+ Añadir proyecto</button>
          {:else}
            <h2>Sin sesiones</h2>
            <p>
              Lanza un agente con <b>+</b> junto a un proyecto
              {#if profiles[0]}o pulsa <kbd>⌘N</kbd> ({profiles[0].id}){/if}.
            </p>
            {#if currentProjectId && profiles.length}
              <div class="quick">
                {#each profiles as prof (prof.id)}
                  <button onclick={() => spawn(currentProjectId!, prof.id)}>{prof.id}</button>
                {/each}
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </div>

    {#if daemonDown}
      <div class="banner">Reconectando con el daemon… <span>{daemonDown}</span></div>
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
  kbd {
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
