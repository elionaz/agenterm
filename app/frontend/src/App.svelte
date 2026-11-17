<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
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
    MAX_PANES,
    assign,
    close as closeLeaf,
    cycle,
    focusedLeaf,
    geometry,
    leaves,
    loadLayout,
    neighbor,
    prune,
    saveLayout,
    sessionsOf,
    setRatio,
    single,
    split,
    type Dir,
    type Divider,
    type Layout,
    type Rect,
  } from './lib/panes'
  import { setAppearance, setTermPrefs, termPrefs } from './lib/prefs.svelte'
  import { shortcutOf, type Toward } from './lib/shortcuts'
  import { DEFAULT_FONT_SIZE, MAX_FONT_SIZE, MIN_FONT_SIZE, themeOf } from './lib/themes'
  import {
    availableUpdate,
    dismissUpdate,
    setAvailableUpdate,
    updateBannerVisible,
    updateNow,
    upgradeCopied,
  } from './lib/updates.svelte'
  import {
    api,
    attentionLabel,
    errorMessage,
    launchable,
    needsAttention,
    rememberedMode,
    sessionTitle,
    type DaemonEvent,
    type Profile,
    type Project,
    type Session,
    type SpawnMode,
    type SpawnOptions,
    type WorktreeInfo,
    type DaemonStatus,
  } from './lib/api'

  let projects = $state<Project[]>([])
  let sessions = $state<Session[]>([])
  let profiles = $state<Profile[]>([])
  /** The focused pane's session. */
  let selectedId = $state<string | null>(null)
  let currentProjectId = $state<string | null>(null)
  let error = $state<string | null>(null)
  let daemonDown = $state<string | null>(null)
  /** The daemon runs another version (left over from before an update). */
  let daemonOld = $state<DaemonStatus | null>(null)
  let restartingDaemon = $state(false)
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
  /** Split-pane layout per project (kept in localStorage). */
  let layouts = $state<Record<string, Layout>>({})
  /** The divider being dragged (its split id). */
  let dragging = $state<string | null>(null)
  /** ⌘F: the focused pane's search bar. */
  let search = $state({ open: false, session: '', query: '', caseSensitive: false, index: -1, count: 0 })
  let searchInput = $state<HTMLInputElement | null>(null)
  let searchOff: (() => void) | null = null

  let panesEl: HTMLDivElement
  let parkingEl: HTMLDivElement
  let errorTimer: number | undefined
  let fontSaveTimer: number | undefined

  const selected = $derived(sessions.find((s) => s.id === selectedId) ?? null)
  const currentProject = $derived(projects.find((p) => p.id === currentProjectId) ?? null)
  const attentionCount = $derived(sessions.filter(needsAttention).length)
  /** What the spawn menus offer, in order (the first one is ⌘N's). */
  const spawnable = $derived(launchable(profiles))
  const selectedTimer = $derived(selected ? timerInfo(selected, now, true) : null)
  /** The selected session has a git folder: offer the Cambios tab. */
  const hasChanges = $derived(!!selected && (selected.mode === 'worktree' || !!selected.git))
  const view = $derived(selected && hasChanges ? (views[selected.id] ?? 'terminal') : 'terminal')

  const layout = $derived(currentProjectId ? (layouts[currentProjectId] ?? null) : null)
  const geo = $derived(layout ? geometry(layout.root) : null)
  const paneList = $derived(layout && geo ? leaves(layout.root).map((leaf) => ({ leaf, rect: geo.rects.get(leaf.id)! })) : [])
  /** Innermost first, so an outer divider is on top where two meet. */
  const dividers = $derived(geo ? [...geo.dividers].reverse() : [])
  const multi = $derived(paneList.length > 1)
  /** Sessions shown in a pane: the user is looking at them. */
  const visibleIds = $derived(layout ? sessionsOf(layout) : [])
  const termBg = $derived(themeOf(termPrefs().theme).colors.background)

  $effect(() => {
    void api.setVisibleSessions(visibleIds).catch(() => {})
  })

  $effect(() => {
    terms.setPrefs(termPrefs())
  })

  function setView(v: 'terminal' | 'changes') {
    if (!selected) return
    views[selected.id] = v
    if (v === 'terminal') terms.focus(selected.id)
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

  /** Acks a session if the user can actually see it (in a pane). */
  function ackIfVisible(id: string | null) {
    if (!id || !document.hasFocus() || !visibleIds.includes(id)) return
    const s = sessions.find((x) => x.id === id)
    if (s && needsAttention(s)) void api.ack(id).catch(() => {})
  }

  function isAppShortcut(e: KeyboardEvent): boolean {
    return shortcutOf(e) !== null
  }

  const terms = new TerminalManager(isAppShortcut, showError, onTermFocus)

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

  // --- Panes ------------------------------------------------------------------

  function setLayout(projectId: string, l: Layout) {
    layouts[projectId] = l
    saveLayout(projectId, l)
  }

  /** The project's layout: the stored one (without sessions that are gone)
   * or a single empty pane. Call once sessions are loaded. */
  function ensureLayout(projectId: string): Layout {
    let l = layouts[projectId]
    if (!l) {
      const stored = loadLayout(projectId)
      l = stored ? prune(stored, new Set(sessions.filter((s) => s.projectId === projectId).map((s) => s.id))) : single(null)
      layouts[projectId] = l
    }
    return l
  }

  /** Drops panes of sessions that no longer exist, in every project. */
  function pruneLayouts() {
    const alive = new Set(sessions.map((s) => s.id))
    for (const [pid, l] of Object.entries(layouts)) {
      const next = prune(l, alive)
      if (next !== l) setLayout(pid, next)
    }
  }

  /** After sessions went away: keep the selection on the focused pane. */
  function reconcileSelection() {
    const l = currentProjectId ? layouts[currentProjectId] : undefined
    const f = l ? focusedLeaf(l).session : null
    if (f && sessions.some((s) => s.id === f)) {
      if (f !== selectedId) select(f, true)
      return
    }
    if (!selectedId || !sessions.some((s) => s.id === selectedId)) select(fallbackSelection(), true)
  }

  function rectStyle(r: Rect): string {
    return `left:${r.x * 100}%;top:${r.y * 100}%;width:${r.w * 100}%;height:${r.h * 100}%`
  }

  function dividerStyle(d: Divider): string {
    const r = d.rect
    return d.dir === 'row'
      ? `left:${(r.x + r.w * d.ratio) * 100}%;top:${r.y * 100}%;height:${r.h * 100}%`
      : `top:${(r.y + r.h * d.ratio) * 100}%;left:${r.x * 100}%;width:${r.w * 100}%`
  }

  /** Puts a session's terminal into a pane's slot (Svelte attachment). */
  function termSlot(id: string) {
    return (node: HTMLElement) => {
      terms.place(id, node)
      return () => terms.unplace(id, node)
    }
  }

  function focusLeaf(leafId: string) {
    const pid = currentProjectId
    const l = pid ? layouts[pid] : undefined
    if (!pid || !l) return
    const leaf = leaves(l.root).find((x) => x.id === leafId)
    if (!leaf) return
    if (l.focus !== leafId) setLayout(pid, { ...l, focus: leafId })
    if (leaf.session) {
      if (leaf.session !== selectedId) select(leaf.session, true)
      else terms.focus(leaf.session)
    } else {
      selectedId = null
    }
  }

  /** A terminal got the keyboard (clicked): its pane becomes the focused one. */
  function onTermFocus(id: string) {
    const pid = currentProjectId
    const l = pid ? layouts[pid] : undefined
    const leaf = l ? leaves(l.root).find((x) => x.session === id) : undefined
    if (!pid || !l || !leaf) return
    if (l.focus !== leaf.id) setLayout(pid, { ...l, focus: leaf.id })
    if (selectedId !== id) {
      if (search.open && search.session !== id) closeSearch(false)
      selectedId = id
      ackIfVisible(id)
    }
  }

  /** The size a terminal gets in a new pane next to the focused one. */
  function splitSize(dir: Dir): { cols: number; rows: number } {
    const cur = selectedId ? terms.size(selectedId) : null
    if (cur) return dir === 'row' ? { cols: Math.max(Math.floor(cur.cols / 2) - 1, 20), rows: cur.rows } : { cols: cur.cols, rows: Math.max(Math.floor(cur.rows / 2) - 1, 5) }
    return paneSize()
  }

  /** The size of a terminal filling the focused pane. */
  function paneSize(): { cols: number; rows: number } {
    const cur = selectedId ? terms.size(selectedId) : null
    if (cur) return cur
    const r = panesEl?.getBoundingClientRect()
    return terms.proposeSize(r?.width ?? 0, r?.height ?? 0)
  }

  /** ⌘D / ⌘⇧D: a new session of the focused one's profile and mode, in a
   * new pane right of / below it. */
  async function splitPane(dir: Dir) {
    const pid = currentProjectId
    if (!pid) return
    const l = ensureLayout(pid)
    if (leaves(l.root).length >= MAX_PANES) {
      showError(t('panes.max', { n: MAX_PANES }))
      return
    }
    const cur = selected && selected.projectId === pid ? selected : null
    const prof = (cur && spawnable.find((p) => p.id === cur.profileId)) || spawnable[0]
    if (!prof) {
      showError(t('panes.noProfile'))
      return
    }
    const mode = cur ? (cur.mode === 'worktree' ? 'worktree' : 'same') : await defaultModeFor(pid)
    const { cols, rows } = splitSize(dir)
    const info = await guard(api.spawn(pid, prof.id, cols, rows, { mode }))
    if (!info) return
    if (!sessions.some((s) => s.id === info.id)) sessions = [...sessions, info]
    const now = layouts[pid] ?? l
    const f = focusedLeaf(now)
    setLayout(pid, f.session ? split(now, f.id, dir, info.id) : assign(now, f.id, info.id))
    select(info.id)
  }

  /** ⌘W: closes the focused pane; its session keeps running. */
  function closePane(leafId?: string) {
    const pid = currentProjectId
    const l = pid ? layouts[pid] : undefined
    if (!pid || !l || leaves(l.root).length <= 1) return
    const next = closeLeaf(l, leafId ?? l.focus)
    setLayout(pid, next)
    if (search.open && !sessionsOf(next).includes(search.session)) closeSearch(false)
    const f = focusedLeaf(next).session
    if (f) select(f, true)
    else selectedId = null
  }

  function movePane(toward: Toward | 1 | -1) {
    const l = currentProjectId ? layouts[currentProjectId] : undefined
    if (!l || leaves(l.root).length <= 1) return
    const target = typeof toward === 'number' ? cycle(l, l.focus, toward) : neighbor(l, l.focus, toward)
    if (target) focusLeaf(target)
  }

  function startDrag(ev: PointerEvent, d: Divider) {
    const pid = currentProjectId
    if (!pid || !panesEl || ev.button !== 0) return
    ev.preventDefault()
    const box = panesEl.getBoundingClientRect()
    const target = ev.currentTarget as HTMLElement
    target.setPointerCapture(ev.pointerId)
    dragging = d.id
    const r = d.rect
    const move = (e: PointerEvent) => {
      const ratio =
        d.dir === 'row' ? ((e.clientX - box.left) / box.width - r.x) / r.w : ((e.clientY - box.top) / box.height - r.y) / r.h
      const l = layouts[pid]
      if (l) layouts[pid] = setRatio(l, d.id, ratio)
    }
    const up = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', up)
      target.removeEventListener('pointercancel', up)
      dragging = null
      const l = layouts[pid]
      if (l) saveLayout(pid, l)
      terms.focus()
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', up)
    target.addEventListener('pointercancel', up)
  }

  // --- Search -----------------------------------------------------------------

  async function openSearch() {
    if (!selectedId || view !== 'terminal') return
    if (search.open && search.session === selectedId) {
      searchInput?.focus()
      searchInput?.select()
      return
    }
    closeSearch(false)
    const id = selectedId
    search.open = true
    search.session = id
    search.index = -1
    search.count = 0
    searchOff = terms.onSearchResults(id, (r) => {
      search.index = r.index
      search.count = r.count
    })
    await tick()
    searchInput?.focus()
    searchInput?.select()
    if (search.query) runSearch(false)
  }

  function runSearch(backwards: boolean, incremental = false) {
    if (!search.open) return
    if (!search.query) {
      terms.clearSearch(search.session)
      search.index = -1
      search.count = 0
      return
    }
    const found = terms.find(search.session, search.query, { caseSensitive: search.caseSensitive, backwards, incremental })
    if (!found) {
      search.index = -1
      search.count = 0
    }
  }

  function closeSearch(refocus = true) {
    if (!search.open) return
    searchOff?.()
    searchOff = null
    terms.clearSearch(search.session)
    search.open = false
    if (refocus) terms.focus(selectedId)
  }

  function onSearchKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      closeSearch()
    } else if (e.key === 'Enter') {
      e.preventDefault()
      runSearch(e.shiftKey)
    }
  }

  const searchStatus = $derived(
    !search.query
      ? ''
      : search.count === 0
        ? t('search.none')
        : search.index >= 0
          ? t('search.count', { i: search.index + 1, n: search.count })
          : t('search.many', { n: search.count }),
  )

  // --- Font size --------------------------------------------------------------

  /** ⌘= / ⌘- / ⌘⌥0: terminal.font_size, applied now, saved shortly after. */
  function bumpFont(delta: 1 | -1 | 0) {
    const p = termPrefs()
    const size = delta === 0 ? DEFAULT_FONT_SIZE : Math.min(Math.max(p.fontSize + delta, MIN_FONT_SIZE), MAX_FONT_SIZE)
    if (size === p.fontSize) return
    setTermPrefs({ ...p, fontSize: size })
    clearTimeout(fontSaveTimer)
    fontSaveTimer = window.setTimeout(() => {
      void api.saveSettings({ terminal: termPrefs() }).catch((e) => showError(errorMessage(e)))
    }, 500)
  }

  // --- Sessions & projects ----------------------------------------------------

  async function refreshSessions() {
    const list = await guard(api.listSessions())
    if (!list) return
    sessions = list
    terms.retain(new Set(list.map((s) => s.id)))
    pruneLayouts()
    if (selectedId && !list.some((s) => s.id === selectedId)) reconcileSelection()
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
    if (!selectedId) {
      // Restore the project's panes (their focused session) if it has any.
      const f = currentProjectId ? focusedLeaf(ensureLayout(currentProjectId)).session : null
      select(f ?? fallbackSelection(), true)
    }
    loaded = true
    refreshAllWorktrees()
    void api
      .getSettings()
      .then((st) => {
        defaultMode = st.git?.defaultMode === 'worktree' ? 'worktree' : 'same'
        setLangSetting(st.ui?.language)
        setAppearance(st.ui?.appearance)
        setTermPrefs(st.terminal)
      })
      .catch(() => {})
    void api
      .daemonStatus()
      .then((s) => (daemonOld = s.mismatch ? s : null))
      .catch(() => {})
  }

  /** Agents in the middle of a turn, which a daemon restart interrupts. */
  const busyCount = $derived(sessions.filter((s) => s.state === 'running').length)
  let confirmRestart = $state(false)

  /** "Restart now": replace the old daemon; its sessions are resumed.
   * Asks first when agents are working. */
  async function restartDaemon(force = false) {
    if (!force && busyCount > 0) {
      confirmRestart = true
      return
    }
    confirmRestart = false
    restartingDaemon = true
    const b = await guard(api.restartDaemon())
    restartingDaemon = false
    if (b) {
      daemonOld = null
      if (b.failed?.length) showError(b.failed.map((f) => `${f.sessionId}: ${f.error}`).join('\n'))
    }
    void refreshAll()
  }

  /** A session of the current project not already shown in a pane. */
  function fallbackSelection(): string | null {
    const shown = new Set(visibleIds)
    const inProject = sessions.filter((s) => s.projectId === currentProjectId && !shown.has(s.id))
    return (inProject.find((s) => s.state !== 'exited') ?? inProject[0] ?? sessions.find((s) => !shown.has(s.id)))?.id ?? null
  }

  /** Shows a session: focuses its pane if it is in one, else puts it in
   * the focused pane. */
  function select(id: string | null, keepGrid = false) {
    if (!keepGrid) gridOpen = false
    const s = id ? sessions.find((x) => x.id === id) : undefined
    if (s) currentProjectId = s.projectId
    const pid = currentProjectId
    if (pid) {
      const l = ensureLayout(pid)
      const hit = id ? leaves(l.root).find((x) => x.session === id) : undefined
      if (hit) {
        if (l.focus !== hit.id) setLayout(pid, { ...l, focus: hit.id })
      } else if (id || !multiPanes(l)) {
        setLayout(pid, assign(l, l.focus, id))
      }
    }
    if (search.open && search.session !== id) closeSearch(false)
    selectedId = id
    terms.focus(id)
    ackIfVisible(id)
  }

  function multiPanes(l: Layout): boolean {
    return leaves(l.root).length > 1
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
    if (selected?.projectId === id) return
    const f = focusedLeaf(ensureLayout(id)).session
    const first = f ?? sessions.find((s) => s.projectId === id)?.id
    if (first) select(first, gridOpen)
    else selectedId = null
  }

  function openSettings() {
    settingsOpen = true
    closeSearch(false)
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
    const { cols, rows } = paneSize()
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

  async function rename(id: string, name: string) {
    const info = await guard(api.rename(id, name))
    if (info) sessions = sessions.map((s) => (s.id === id ? { ...s, name: info.name } : s))
  }

  async function resume(id: string) {
    const info = await guard(api.resume(id))
    if (info) sessions = sessions.map((s) => (s.id === id ? info : s))
  }

  async function discard(id: string) {
    await guard(api.discard(id)) // session.removed updates the list
  }

  async function resumeAll() {
    const b = await guard(api.resumeAll())
    if (b?.failed?.length) showError(b.failed.map((f) => `${f.sessionId}: ${f.error}`).join('\n'))
    void refreshSessions()
  }

  async function discardAll() {
    await guard(api.discardAll())
    void refreshSessions()
  }

  async function addProject() {
    const p = await guard(api.addProject())
    if (!p) return
    if (!projects.some((x) => x.id === p.id)) projects = [...projects, p]
    currentProjectId = p.id
  }

  async function removeProject(id: string) {
    // Its live sessions would keep running with no row left to reach them.
    const live = sessions.filter((s) => s.projectId === id && s.state !== 'exited').length
    if (live) {
      showError(t('sidebar.removeProjectLive', { n: live }))
      return
    }
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
        if (ev.raised) ackIfVisible(ev.sessionId)
        break
      case 'session.activity':
        sessions = sessions.map((s) => (s.id === ev.sessionId ? mergeLive(s, ev) : s))
        break
      case 'session.git':
        sessions = sessions.map((s) => (s.id === ev.sessionId ? { ...s, git: ev.git as Session['git'] } : s))
        break
      case 'session.renamed':
        sessions = sessions.map((s) => (s.id === ev.sessionId ? { ...s, name: ev.sessionName } : s))
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
        if (search.session === ev.sessionId) closeSearch(false)
        pruneLayouts()
        if (selectedId === ev.sessionId) reconcileSelection()
        break
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && orphanView && !settingsOpen) {
      e.preventDefault()
      closeOrphan()
      return
    }
    const sc = shortcutOf(e)
    if (!sc) return
    e.preventDefault()
    if (sc.kind === 'settings') {
      openSettings()
      return
    }
    if (sc.kind === 'font') {
      bumpFont(sc.delta)
      return
    }
    if (settingsOpen) return
    const covered = gridOpen || !!orphanView
    switch (sc.kind) {
      case 'grid':
        if (gridOpen) closeGrid()
        else openGrid()
        return
      case 'session': {
        const list = sessions.filter((s) => s.projectId === currentProjectId)
        if (list[sc.n - 1]) select(list[sc.n - 1].id)
        return
      }
      case 'new': {
        // ⌘N: new session in the current project with the first profile.
        const pid = currentProjectId ?? projects[0]?.id
        const prof = spawnable[0]
        if (pid && prof) void spawnDefault(pid, prof.id)
        return
      }
    }
    if (covered) return
    switch (sc.kind) {
      case 'split':
        void splitPane(sc.dir)
        return
      case 'closePane':
        closePane()
        return
      case 'cyclePane':
        movePane(sc.delta)
        return
      case 'movePane':
        movePane(sc.toward)
        return
      case 'find':
        void openSearch()
        return
      case 'findNext':
        if (search.open) runSearch(sc.backwards)
        else if (search.query) void openSearch().then(() => runSearch(sc.backwards))
        return
    }
  }

  /** The remembered (or default) mode, but like the + menu, the same
   * folder when the project cannot use worktrees (not a git repo…). */
  async function defaultModeFor(projectId: string): Promise<SpawnMode> {
    const mode = rememberedMode(projectId, defaultMode)
    if (mode === 'worktree') {
      const g = await api.projectGit(projectId).catch(() => null)
      if (!g?.isRepo || g.error) return 'same'
    }
    return mode
  }

  async function spawnDefault(projectId: string, profileId: string) {
    await spawn(projectId, profileId, { mode: await defaultModeFor(projectId) })
  }

  function focusFromNotification(id: string) {
    if (sessions.some((s) => s.id === id)) select(id)
    else void refreshSessions().then(() => sessions.some((s) => s.id === id) && select(id))
  }

  const offs: Array<() => void> = []
  onMount(() => {
    terms.mount(parkingEl)
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
      // A newer release was found (null: none any more).
      EventsOn('updates.available', setAvailableUpdate),
    )
    // One found before the window loaded (the event may also win the race).
    void api
      .updaterState()
      .then((s) => s.available && setAvailableUpdate(s.available))
      .catch(() => {})
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
      for (const id of visibleIds) ackIfVisible(id)
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
    {visibleIds}
    {currentProjectId}
    onSelect={select}
    onSelectProject={selectProject}
    onSpawn={spawn}
    onKill={kill}
    onRename={rename}
    onResume={resume}
    onDiscard={discard}
    onResumeAll={resumeAll}
    onDiscardAll={discardAll}
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
        <span class="title">{sessionTitle(selected)}</span>
        {#if selected.name}<span class="profile">{selected.profileId}</span>{/if}
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
      <div class="panes" class:multi class:dragging={!!dragging} bind:this={panesEl}>
        {#each paneList as p (p.leaf.id)}
          {@const s = p.leaf.session ? sessions.find((x) => x.id === p.leaf.session) : undefined}
          {@const focused = p.leaf.id === layout?.focus}
          {@const prog = s && s.state !== 'exited' ? progressOf(s) : undefined}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="pane" class:focused style={rectStyle(p.rect)} onmousedown={() => !focused && focusLeaf(p.leaf.id)}>
            <div class="pane-frame">
              {#if multi}
                <div class="pane-head" title={s ? `${s.cmd} · ${s.cwd} · ${s.id}` : ''}>
                  <span class="pane-name">{s ? sessionTitle(s) : t('panes.empty')}</span>
                  {#if s?.name}<span class="pane-profile">{s.profileId}</span>{/if}
                  {#if s && needsAttention(s)}<span class="attn {s.attentionKind}">{attentionLabel(s.attentionKind)}</span>{/if}
                  <button
                    class="pane-close"
                    title={t('panes.close')}
                    aria-label={t('panes.close')}
                    onmousedown={(e) => e.stopPropagation()}
                    onclick={() => closePane(p.leaf.id)}>×</button
                  >
                </div>
              {/if}
              <div class="pane-body" style:background={termBg}>
                {#if prog}<div class="pane-progress"><ProgressBar progress={prog} thick /></div>{/if}
                {#if p.leaf.session}
                  {#key p.leaf.session}
                    <div class="term-slot" {@attach termSlot(p.leaf.session)}></div>
                  {/key}
                {:else if multi}
                  <div class="pane-empty">{t('panes.emptyHint')}</div>
                {/if}
                {#if focused && selected && view === 'changes'}
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
                {#if focused && search.open && search.session === p.leaf.session}
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div class="search" onmousedown={(e) => e.stopPropagation()}>
                    <input
                      bind:this={searchInput}
                      bind:value={search.query}
                      placeholder={t('search.placeholder')}
                      spellcheck="false"
                      oninput={() => runSearch(false, true)}
                      onkeydown={onSearchKey}
                    />
                    <span class="search-status">{searchStatus}</span>
                    <button
                      class="search-btn"
                      class:on={search.caseSensitive}
                      title={t('search.caseSensitive')}
                      aria-pressed={search.caseSensitive}
                      onclick={() => {
                        search.caseSensitive = !search.caseSensitive
                        // The addon keeps its highlights for the same term:
                        // drop them so the matches are recounted.
                        terms.clearSearch(search.session)
                        runSearch(false)
                        searchInput?.focus()
                      }}>Aa</button
                    >
                    <button class="search-btn" title={t('search.prev')} aria-label={t('search.prev')} onclick={() => runSearch(true)}>↑</button>
                    <button class="search-btn" title={t('search.next')} aria-label={t('search.next')} onclick={() => runSearch(false)}>↓</button>
                    <button class="search-btn" title={t('search.close')} aria-label={t('search.close')} onclick={() => closeSearch()}>×</button>
                  </div>
                {/if}
              </div>
            </div>
          </div>
        {/each}
        {#each dividers as d (d.id)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="divider {d.dir}"
            class:active={dragging === d.id}
            style={dividerStyle(d)}
            onpointerdown={(e) => startDrag(e, d)}
          ></div>
        {/each}
      </div>
      <div class="parking" bind:this={parkingEl}></div>

      {#if loaded && !selected && !multi}
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
    {:else if daemonOld}
      <div class="banner action">
        {#if confirmRestart && busyCount > 0}
          {t('banner.restartBusy', { n: busyCount })}
          <button onclick={() => restartDaemon(true)}>{t('banner.restartAnyway')}</button>
          <button onclick={() => (confirmRestart = false)}>{t('banner.cancel')}</button>
        {:else}
          {daemonOld.daemonVersion
            ? t('banner.daemonOld', { version: daemonOld.daemonVersion })
            : t('banner.daemonOldUnknown')}
          <button disabled={restartingDaemon} onclick={() => restartDaemon()}>
            {restartingDaemon ? t('banner.restarting') : t('banner.restartNow')}
          </button>
        {/if}
      </div>
    {:else if updateBannerVisible()}
      {@const u = availableUpdate()!}
      <div class="banner action">
        {#if u.brew}
          {t('banner.updateBrew', { version: u.version })}
          <button onclick={() => void guard(updateNow())}>
            {upgradeCopied() ? t('banner.updateCopied') : t('banner.updateCopy')}
          </button>
        {:else if u.ready}
          {t('banner.updateReady', { version: u.version })}
          <button onclick={() => void guard(updateNow())}>{t('banner.updateRestart')}</button>
        {:else}
          {t('banner.updateAvailable', { version: u.version })}
          <button onclick={() => void guard(updateNow())}>{t('banner.update')}</button>
        {/if}
        <button onclick={dismissUpdate}>{t('banner.later')}</button>
      </div>
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
  .profile {
    color: var(--muted);
    font-size: 11.5px;
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
    color: var(--orange);
    font-weight: 600;
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

  /* Split panes: absolutely placed by their rect in the tree. */
  .panes {
    position: absolute;
    inset: 0;
  }
  .panes.dragging {
    user-select: none;
  }
  .panes.dragging .pane {
    pointer-events: none;
  }
  .pane {
    position: absolute;
    display: flex;
  }
  .multi .pane {
    padding: 3px;
  }
  .pane-frame {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    border-radius: 4px;
    overflow: hidden;
  }
  .multi .pane-frame {
    border: 1px solid var(--border);
  }
  .multi .pane.focused .pane-frame {
    border-color: color-mix(in srgb, var(--blue) 70%, transparent);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--blue) 35%, transparent);
  }
  .pane-head {
    flex: none;
    height: 22px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 4px 0 8px;
    font-size: 11.5px;
    color: var(--muted);
    background: var(--sidebar-bg);
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
    overflow: hidden;
  }
  .pane.focused .pane-head {
    color: var(--fg);
  }
  .pane-name {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pane-profile {
    color: var(--muted);
  }
  .pane-head .attn {
    padding: 0 6px;
    font-size: 10.5px;
  }
  .pane-close {
    margin-left: auto;
    width: 18px;
    height: 18px;
    border-radius: 4px;
    line-height: 1;
    font-size: 14px;
    color: var(--muted);
  }
  .pane-close:hover {
    background: var(--hover-strong);
    color: var(--fg);
  }
  .pane-body {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .term-slot {
    position: absolute;
    inset: 4px 0 0 8px;
  }
  .multi .term-slot {
    inset: 3px 0 0 6px;
  }
  .term-slot :global(.term-instance),
  .parking :global(.term-instance) {
    position: absolute;
    inset: 0;
  }
  .parking {
    display: none;
  }
  .pane-progress {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    z-index: 2;
  }
  .pane-empty {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    text-align: center;
    color: var(--muted);
    font-size: 12px;
  }
  .divider {
    position: absolute;
    z-index: 4;
  }
  .divider.row {
    width: 8px;
    margin-left: -4px;
    cursor: col-resize;
  }
  .divider.col {
    height: 8px;
    margin-top: -4px;
    cursor: row-resize;
  }
  .divider:hover,
  .divider.active {
    background: color-mix(in srgb, var(--blue) 30%, transparent);
  }

  /* ⌘F search bar, top right of the focused pane. */
  .search {
    position: absolute;
    top: 6px;
    right: 14px;
    z-index: 6;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 4px 4px 6px;
    border-radius: 6px;
    background: var(--menu-bg);
    border: 1px solid var(--border-strong);
    box-shadow: 0 6px 18px var(--shadow);
  }
  .search input {
    width: 190px;
    padding: 3px 6px;
    font: 12px var(--mono);
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    outline: none;
  }
  .search input:focus {
    border-color: var(--blue);
  }
  .search-status {
    min-width: 54px;
    color: var(--muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    text-align: right;
  }
  .search-btn {
    min-width: 22px;
    height: 22px;
    padding: 0 4px;
    border-radius: 4px;
    color: var(--fg-dim);
    font-size: 12px;
  }
  .search-btn:hover {
    background: var(--hover-strong);
    color: var(--fg);
  }
  .search-btn.on {
    background: var(--accent-bg);
    color: var(--fg);
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
    background: var(--warn-bg);
    color: var(--amber);
    font-size: 12px;
  }
  .banner span {
    color: var(--muted);
    margin-left: 6px;
  }
  .banner.action {
    display: flex;
    align-items: center;
    gap: 10px;
    max-width: 70%;
  }
  .banner.action button {
    flex: none;
    padding: 3px 10px;
    border: 1px solid var(--amber);
    border-radius: 5px;
    background: transparent;
    color: var(--amber);
    font: inherit;
    cursor: pointer;
  }
  .banner.action button:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .toast {
    position: absolute;
    right: 14px;
    bottom: 14px;
    z-index: 30;
    max-width: 420px;
    padding: 8px 12px;
    border-radius: 6px;
    background: var(--danger-bg);
    color: var(--danger-fg);
    border: 1px solid var(--danger-border);
    font-size: 12px;
    cursor: default;
  }
</style>
