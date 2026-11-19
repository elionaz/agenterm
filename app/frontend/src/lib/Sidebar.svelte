<script lang="ts">
  import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
  import { permHelp } from './permission'
  import {
    api,
    attentionLabel,
    changesLabel,
    needsAttention,
    rememberMode,
    displayArgs,
    rememberedMode,
    sessionTitle,
    type Profile,
    type Project,
    type ProjectGit,
    type Session,
    type SpawnMode,
    type SpawnOptions,
    type WorktreeInfo,
  } from './api'
  import { activityText, progressOf, shortTimer, stateLabel } from './activity'
  import { hasKey, t } from './i18n.svelte'
  import GridIcon from './GridIcon.svelte'
  import ProgressBar from './ProgressBar.svelte'
  import ProjectBadge from './ProjectBadge.svelte'
  import ProjectCustomize from './ProjectCustomize.svelte'
  import type { ProjectPatch } from './badge'
  import { dropSlot, moveTo } from './reorder'
  import { SHELL_PROFILE, folderSharers, inProjectFolder, joinNames } from './shared'
  import { proposedMode } from './spawnmode'
  import { prettyModel } from './models'
  import { branchParts } from './branch'
  import { clampSidebar, loadSidebarWidth, saveSidebarWidth, SIDEBAR_DEFAULT } from './sidebarwidth'

  interface Props {
    projects: Project[]
    sessions: Session[]
    profiles: Profile[]
    selectedId: string | null
    /** Sessions shown in a split pane (the selected one included). */
    visibleIds: string[]
    currentProjectId: string | null
    onSelect: (id: string) => void
    /** Renames a session ('' resets it to the profile's name). */
    onRename: (id: string, name: string) => void
    onSelectProject: (id: string) => void
    onSpawn: (projectId: string, profileId: string, opts?: Partial<SpawnOptions>) => void
    /** agenterm worktrees per project (orphans are listed under it). */
    worktrees: Record<string, WorktreeInfo[]>
    /** Opens an orphaned worktree's changes (optionally with removal asked). */
    onOpenWorktree: (worktreeId: string, remove?: boolean) => void
    onKill: (id: string) => void
    /** Suspended sessions (restored after a daemon restart). */
    onResume: (id: string) => void
    onDiscard: (id: string) => void
    onResumeAll: () => void
    onDiscardAll: () => void
    onAddProject: () => void
    onRemoveProject: (id: string) => void
    /** Drag & drop: the new project order. */
    onReorderProjects: (ids: string[]) => void
    /** Renames a project or changes its badge; rejects with a message. */
    onUpdateProject: (id: string, patch: ProjectPatch) => Promise<void>
    /** Drag & drop (or ⌘⇧↑/↓): the new order of a project's sessions. */
    onReorderSessions: (projectId: string, ids: string[]) => void
    onOpenSettings: () => void
    /** Opens the grid overview (focused on a project when given). */
    onOpenGrid: (projectId?: string) => void
    /** The overview of every project is open (its row is highlighted). */
    overviewOpen?: boolean
    /** Current time (ms), ticking once per second, for the timers. */
    now: number
  }

  let {
    projects,
    sessions,
    profiles,
    selectedId,
    visibleIds,
    currentProjectId,
    onSelect,
    onRename,
    onSelectProject,
    onSpawn,
    onKill,
    onResume,
    onDiscard,
    onResumeAll,
    onDiscardAll,
    onAddProject,
    overviewOpen = false,
    onRemoveProject,
    onReorderProjects,
    onUpdateProject,
    onReorderSessions,
    onOpenSettings,
    onOpenGrid,
    now,
    worktrees,
    onOpenWorktree,
  }: Props = $props()

  let collapsed = $state<Record<string, boolean>>({})
  /** The project whose customize popover is open. */
  let customizing = $state<string | null>(null)

  function openCustomize(projectId: string, ev: MouseEvent) {
    ev.preventDefault()
    ev.stopPropagation()
    closeMenu()
    customizing = customizing === projectId ? null : projectId
  }

  /** Closes the popover and gives the focus back to the project's badge. */
  function closeCustomize() {
    const id = customizing
    customizing = null
    if (id) document.querySelector<HTMLElement>(`[data-badge="${CSS.escape(id)}"]`)?.focus()
  }
  /** The session whose name is being edited, and the text typed. */
  let renaming = $state<string | null>(null)
  let renameText = $state('')

  function startRename(s: Session) {
    renaming = s.id
    renameText = s.name ?? ''
  }

  function finishRename(save: boolean) {
    const id = renaming
    if (!id) return
    renaming = null
    const s = sessions.find((x) => x.id === id)
    const name = renameText.trim()
    if (save && s && name !== (s.name ?? '')) onRename(id, name)
  }

  function onRenameKey(e: KeyboardEvent) {
    e.stopPropagation() // keep Escape/Enter away from the window handlers
    if (e.key === 'Enter') {
      e.preventDefault()
      finishRename(true)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      finishRename(false)
    }
  }

  /** Focuses and selects the rename field once it is shown. */
  function autofocus(node: HTMLInputElement) {
    node.focus()
    node.select()
  }
  let menuFor = $state<string | null>(null)
  /** Second step of the spawn menu: the profile picked, where to run it. */
  let picked = $state<Profile | null>(null)
  let projGit = $state<ProjectGit | null>(null)
  let mode = $state<SpawnMode>('same')
  let branch = $state('')
  let base = $state('')
  let defaultMode: SpawnMode = 'worktree'

  // Drag & drop: a project among projects, or a session among its project's
  // sessions. dropAt is the gap the pointer is over (see reorder.ts).
  let drag = $state<{ kind: 'project' | 'session'; id: string; projectId: string } | null>(null)
  let dropAt = $state<{ kind: 'project' | 'session'; projectId: string; slot: number } | null>(null)

  function onDragStart(ev: DragEvent, kind: 'project' | 'session', id: string, projectId: string) {
    if ((ev.target as HTMLElement).closest('button, input')) {
      ev.preventDefault()
      return
    }
    ev.stopPropagation()
    drag = { kind, id, projectId }
    ev.dataTransfer?.setData('text/plain', id)
    if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'move'
  }

  function onDragOver(ev: DragEvent, kind: 'project' | 'session', projectId: string, i: number) {
    if (!drag || drag.kind !== kind || (kind === 'session' && drag.projectId !== projectId)) return
    ev.preventDefault()
    ev.stopPropagation()
    if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move'
    dropAt = { kind, projectId, slot: dropSlot(ev.clientY, (ev.currentTarget as HTMLElement).getBoundingClientRect(), i) }
  }

  function onDrop(ev: DragEvent) {
    if (!drag || !dropAt || drag.kind !== dropAt.kind) return onDragEnd()
    ev.preventDefault()
    ev.stopPropagation()
    const { kind, id, projectId } = drag
    const slot = dropAt.slot
    onDragEnd()
    if (kind === 'project') {
      const next = moveTo(projects.map((p) => p.id), id, slot)
      if (next) onReorderProjects(next)
    } else {
      const next = moveTo((byProject.get(projectId) ?? []).map((s) => s.id), id, slot)
      if (next) onReorderSessions(projectId, next)
    }
  }

  function onDragEnd() {
    drag = null
    dropAt = null
  }

  /** The drop line goes before item i, or after it when it is the last. */
  const dropBefore = (kind: 'project' | 'session', projectId: string, i: number) =>
    dropAt?.kind === kind && dropAt.projectId === projectId && dropAt.slot === i
  const dropAfter = (kind: 'project' | 'session', projectId: string, i: number, n: number) =>
    dropAt?.kind === kind && dropAt.projectId === projectId && dropAt.slot === i + 1 && i === n - 1

  const byProject = $derived.by(() => {
    const m = new Map<string, Session[]>()
    for (const s of sessions) {
      const list = m.get(s.projectId) ?? []
      list.push(s)
      m.set(s.projectId, list)
    }
    return m
  })

  function badgeClass(s: Session): string {
    if (s.state === 'exited') return s.exitCode ? 'exited fail' : 'exited'
    if (needsAttention(s)) return `${s.state} attention ${s.attentionKind}`
    return s.state
  }

  // Width: drag the right edge (double-click: back to the default).
  let width = $state(loadSidebarWidth())
  let resizing: { x: number; w: number } | null = null
  function resizeStart(e: PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    resizing = { x: e.clientX, w: width }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }
  function resizeMove(e: PointerEvent) {
    if (resizing) width = clampSidebar(resizing.w + e.clientX - resizing.x)
  }
  function resizeEnd() {
    if (!resizing) return
    resizing = null
    saveSidebarWidth(width)
  }
  function resizeReset() {
    width = SIDEBAR_DEFAULT
    saveSidebarWidth(width)
  }

  /** The state word, when it says more than the dot's color: attention,
   * a wait, an exit or a suspended session. Working / idle are in the
   * tooltip. */
  function showsState(s: Session): boolean {
    return needsAttention(s) || s.state === 'waiting_input' || s.state === 'exited' || s.state === 'suspended'
  }

  const suspendedCount = $derived(sessions.filter((s) => s.state === 'suspended').length)

  function tooltip(s: Session): string {
    let base = `${stateLabel(s)} · ${[s.cmd, displayArgs(s.args)].filter(Boolean).join(' ')} · ${s.cwd} · ${s.id}`
    if (permHelp(s.permission)) base = `${permHelp(s.permission)}\n${base}`
    if (s.state === 'suspended') {
      const at = s.suspendedAt ? new Date(s.suspendedAt).toLocaleString() : ''
      base = `${t('sidebar.suspendedSince', { at })}\n${base}`
      if (s.resumeError) base = `${t('sidebar.resumeFailed', { msg: s.resumeError })}\n${base}`
      return base
    }
    if (s.activity) base = `${activityText(s)}\n${base}`
    if (!s.attentionKind || s.state === 'exited') return base
    const what = attentionLabel(s.attentionKind)
    return `${what}${s.attentionMessage ? `: ${s.attentionMessage}` : ''}\n${base}`
  }

  function toggleMenu(projectId: string, ev: MouseEvent) {
    ev.stopPropagation()
    customizing = null
    if (menuFor === projectId) {
      closeMenu()
      return
    }
    menuFor = projectId
    picked = null
    projGit = null
    // What the second step needs: is it a repo, its branches, the default.
    void api
      .projectGit(projectId)
      .then((g) => menuFor === projectId && (projGit = g))
      .catch(() => menuFor === projectId && (projGit = { isRepo: false, branches: [], error: t('git.unavailable') }))
    void api
      .getSettings()
      .then((st) => (defaultMode = st.git?.defaultMode === 'same' ? 'same' : 'worktree'))
      .catch(() => {})
  }

  const canWorktree = $derived(!!projGit?.isRepo && !projGit.error)
  /** Live agents already in the folder of the project whose menu is open. */
  const menuSharers = $derived(menuFor ? folderSharers(sessions, menuFor) : [])
  /** Suggest a worktree: an agent (not a plain shell) is being started where
   * another one already works. */
  const suggestWorktree = $derived(!!picked && picked.id !== SHELL_PROFILE && menuSharers.length > 0)

  // The repo info can arrive after the profile was picked: no worktree
  // outside git, and the base it reports.
  $effect(() => {
    if (!picked || !projGit) return
    if (mode === 'worktree' && !canWorktree) mode = 'same'
    if (!base) base = projGit.defaultBase || projGit.currentBranch || ''
  })

  function names(list: Session[]): string {
    return joinNames(list.map(sessionTitle))
  }

  /** The ±N tooltip: a session in the project folder shares it with the others there. */
  function changesTitle(s: Session): string {
    const n = s.git?.files ?? 0
    const others = inProjectFolder(s) ? folderSharers(sessions, s.projectId, s.id) : []
    return others.length ? t('sidebar.sharedTitle', { n, names: names(others) }) : t('sidebar.filesChanged', { n })
  }

  function randomSuffix(): string {
    return Math.random().toString(16).slice(2, 6)
  }

  function pick(projectId: string, prof: Profile) {
    picked = prof
    // Another agent already works in the project folder: suggest a worktree,
    // so the two don't share a branch and changes.
    const shell = prof.id === SHELL_PROFILE
    const busy = !shell && folderSharers(sessions, projectId).length > 0
    mode = proposedMode({ shell, busy, remembered: rememberedMode(projectId, defaultMode), fallback: defaultMode })
    if (mode === 'worktree' && projGit && !canWorktree) mode = 'same'
    branch = `agent/${prof.id}-${randomSuffix()}`
    base = projGit?.defaultBase || projGit?.currentBranch || ''
  }

  function launch(projectId: string) {
    if (!picked) return
    const wt = mode === 'worktree' && canWorktree
    // A suggested worktree (the folder was busy) is not a preference, and a
    // shell always starts in the folder.
    if (!suggestWorktree && picked.id !== SHELL_PROFILE) rememberMode(projectId, wt ? 'worktree' : 'same')
    const prof = picked
    closeMenu()
    onSpawn(projectId, prof.id, wt ? { mode: 'worktree', branch: branch.trim(), base } : { mode: 'same' })
  }

  function closeMenu() {
    menuFor = null
    picked = null
  }

  function orphansOf(projectId: string): WorktreeInfo[] {
    return (worktrees[projectId] ?? []).filter((w) => w.orphan)
  }

  function reopen(projectId: string, w: WorktreeInfo) {
    const shell = profiles.find((p) => p.id === 'shell') ?? profiles[0]
    if (!shell) return
    onSpawn(projectId, shell.id, { worktreeId: w.id })
  }

  function branchTitle(s: Session): string {
    let out = t('sidebar.worktreeOn', { branch: s.git?.head || s.branch || '' })
    if (s.git?.head && s.branch && s.git.head !== s.branch) out += ` (${t('changes.startedOn', { branch: s.branch }).replace(/^· /, '')})`
    if (s.base) out += t('sidebar.worktreeFrom', { base: s.base })
    if (s.git) {
      out += `\n${t('sidebar.filesChanged', { n: s.git.files })} · ${t('git.uncommitted', { n: s.git.uncommitted })} · ${t('git.commitsAhead', { n: s.git.ahead })}`
    }
    return out
  }

  /** Why worktrees are unavailable, in the UI language when known. */
  function gitReason(g: ProjectGit | null): string {
    const key = `git.${g?.errorCode ?? ''}`
    if (g?.errorCode && hasKey(key)) return t(key)
    return g?.error || t('git.not_a_repo')
  }
</script>

{#snippet branchText(branch: string)}
  {@const p = branchParts(branch)}
  <span class="bh">⎇ {p.head}</span>{#if p.tail}<span class="bt">{p.tail}</span>{/if}
{/snippet}

<svelte:window
  onclick={() => {
    closeMenu()
    customizing = null
  }}
  onkeydown={(e) => {
    if (e.key !== 'Escape') return
    if (menuFor) closeMenu()
    if (customizing) closeCustomize()
  }}
/>

<aside class="sidebar" style:width="{width}px" style:min-width="{width}px">
  <div class="titlebar drag">
    <span class="brand">agenterm</span>
  </div>

  <div class="nav">
    <button class="overview" class:active={overviewOpen} title={t('sidebar.overview')} onclick={() => onOpenGrid()}>
      <span class="overview-icon"><GridIcon size={13} /></span>
      <span class="overview-label">{t('sidebar.overviewLabel')}</span>
      <kbd>⌘0</kbd>
    </button>
  </div>

  {#if suspendedCount}
    <div class="restored" role="status">
      <span>{t('sidebar.restoredBanner', { n: suspendedCount })}</span>
      <span class="restored-actions">
        <button onclick={onResumeAll}>{t('sidebar.resumeAll')}</button>
        <button class="danger" onclick={onDiscardAll}>{t('sidebar.discardAll')}</button>
      </span>
    </div>
  {/if}

  <div class="projects">
    {#each projects as p, pi (p.id)}
      {@const list = byProject.get(p.id) ?? []}
      {@const pending = list.filter(needsAttention).length}
      <section class="project" class:current={p.id === currentProjectId}>
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="project-row"
          class:dragging={drag?.kind === 'project' && drag.id === p.id}
          class:drop-before={dropBefore('project', '', pi)}
          class:drop-after={dropAfter('project', '', pi, projects.length)}
          title={p.path}
          draggable={customizing !== p.id}
          ondragstart={(e) => onDragStart(e, 'project', p.id, '')}
          ondragover={(e) => onDragOver(e, 'project', '', pi)}
          ondrop={onDrop}
          ondragend={onDragEnd}
          onclick={() => onSelectProject(p.id)}
          oncontextmenu={(e) => openCustomize(p.id, e)}
        >
          <button
            class="chevron"
            class:closed={collapsed[p.id]}
            aria-label={collapsed[p.id] ? t('sidebar.expand') : t('sidebar.collapse')}
            onclick={(e) => {
              e.stopPropagation()
              collapsed[p.id] = !collapsed[p.id]
            }}>▾</button
          >
          <button
            class="badge-btn"
            data-badge={p.id}
            aria-expanded={customizing === p.id}
            title={t('sidebar.customizeProject')}
            aria-label={t('project.customize')}
            onclick={(e) => openCustomize(p.id, e)}><ProjectBadge project={p} /></button
          >
          <span class="project-name">{p.name}</span>
          {#if pending}
            <span class="attn-count" title={t('sidebar.needAttention', { n: pending })}
              >{pending}</span
            >
          {:else if list.length}<span class="count">{list.length}</span>{/if}
          <span class="row-actions">
            <button class="icon" title={t('sidebar.projectOverview')} aria-label={t('sidebar.projectOverview')} onclick={(e) => {
              e.stopPropagation()
              onOpenGrid(p.id)
            }}><GridIcon size={12} /></button>
            <button class="icon" title={t('sidebar.removeProject')} onclick={(e) => {
              e.stopPropagation()
              onRemoveProject(p.id)
            }}>−</button>
            <button class="icon" title={t('sidebar.newSession')} onclick={(e) => toggleMenu(p.id, e)}>+</button>
          </span>
          {#if customizing === p.id}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="menu wide"
              role="dialog"
              aria-label={t('project.customize')}
              tabindex="-1"
              title=""
              onclick={(e) => e.stopPropagation()}
              oncontextmenu={(e) => e.stopPropagation()}
            >
              <ProjectCustomize
                project={p}
                onSave={(patch) => onUpdateProject(p.id, patch)}
                onClose={closeCustomize}
              />
            </div>
          {/if}
          {#if menuFor === p.id}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="menu" class:wide={!!picked} onclick={(e) => e.stopPropagation()}>
              {#if !picked}
                {#each profiles as prof (prof.id)}
                  <button class="menu-item" title={prof.path ?? prof.cmd} onclick={() => pick(p.id, prof)}>
                    <span>{prof.name || prof.id}</span>
                    <span class="menu-cmd">{prof.id}</span>
                  </button>
                {:else}
                  <div class="menu-empty">{t('sidebar.noAgents')}</div>
                {/each}
                <div class="menu-sep"></div>
                <button
                  class="menu-item muted"
                  onclick={() => {
                    closeMenu()
                    onOpenSettings()
                  }}><span>{t('sidebar.chooseAgents')}</span><span class="menu-cmd">⌘,</span></button
                >
              {:else}
                <form
                  class="spawn-form"
                  onsubmit={(e) => {
                    e.preventDefault()
                    launch(p.id)
                  }}
                >
                  <div class="spawn-head">
                    <button type="button" class="icon back" title={t('sidebar.back')} onclick={() => (picked = null)}>‹</button>
                    <b>{picked.name || picked.id}</b>
                  </div>
                  <label class="opt">
                    <input type="radio" bind:group={mode} value="same" />
                    <span><b>{t('sidebar.sameFolder')}</b><em>{p.path}</em></span>
                  </label>
                  <label
                    class="opt"
                    class:disabled={!canWorktree}
                    title={!projGit ? t('sidebar.checkingRepo') : canWorktree ? '' : t('sidebar.unavailable', { reason: gitReason(projGit) })}
                  >
                    <input type="radio" bind:group={mode} value="worktree" disabled={!canWorktree} />
                    <span
                      ><b>{t('sidebar.newWorktree')}</b><em
                        >{!projGit ? t('sidebar.checkingGit') : canWorktree ? t('sidebar.ownBranch') : gitReason(projGit)}</em
                      ></span
                    >
                  </label>
                  {#if suggestWorktree && canWorktree}
                    <p class="shared-hint" class:warn={mode === 'same'}>
                      {t('sidebar.sharedHint', { n: menuSharers.length, names: names(menuSharers) })}
                    </p>
                  {/if}
                  {#if mode === 'worktree' && canWorktree}
                    <label class="fld">
                      <span>{t('sidebar.branch')}</span>
                      <input bind:value={branch} spellcheck="false" placeholder="agent/…" />
                    </label>
                    <label class="fld">
                      <span>{t('sidebar.base')}</span>
                      <select bind:value={base}>
                        {#each projGit?.branches ?? [] as b (b)}
                          <option value={b}>{b}{b === projGit?.currentBranch ? t('sidebar.current') : ''}</option>
                        {/each}
                      </select>
                    </label>
                  {/if}
                  <div class="spawn-actions">
                    <button type="submit" class="launch">{t('sidebar.launch')}</button>
                  </div>
                </form>
              {/if}
            </div>
          {/if}
        </div>

        {#if !collapsed[p.id]}
          <ul class="sessions">
            {#each list as s, i (s.id)}
              {@const timer = shortTimer(s, now)}
              {@const prog = s.state !== 'exited' ? progressOf(s) : undefined}
              {@const head = s.mode === 'worktree' && s.branch ? s.git?.head || s.branch : s.git?.head}
              <li>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="session"
                  class:dragging={drag?.kind === 'session' && drag.id === s.id}
                  class:drop-before={dropBefore('session', p.id, i)}
                  class:drop-after={dropAfter('session', p.id, i, list.length)}
                  draggable={renaming !== s.id}
                  ondragstart={(e) => onDragStart(e, 'session', s.id, p.id)}
                  ondragover={(e) => onDragOver(e, 'session', p.id, i)}
                  ondrop={onDrop}
                  ondragend={onDragEnd}
                  class:selected={s.id === selectedId}
                  class:shown={s.id !== selectedId && visibleIds.includes(s.id)}
                  class:needs={needsAttention(s)}
                  title={tooltip(s)}
                  onclick={() => renaming !== s.id && onSelect(s.id)}
                  oncontextmenu={(e) => {
                    e.preventDefault()
                    startRename(s)
                  }}
                >
                  <div class="line">
                    <span class="dot {badgeClass(s)}"></span>
                    {#if renaming === s.id}
                      <input
                        class="rename"
                        bind:value={renameText}
                        placeholder={s.profileId}
                        maxlength="80"
                        spellcheck="false"
                        aria-label={t('sidebar.renameLabel')}
                        {@attach autofocus}
                        onkeydown={onRenameKey}
                        onblur={() => finishRename(true)}
                        onclick={(e) => e.stopPropagation()}
                        ondblclick={(e) => e.stopPropagation()}
                      />
                    {:else}
                      <!-- svelte-ignore a11y_no_static_element_interactions -->
                      <span
                        class="session-name"
                        title={t('sidebar.renameHint')}
                        ondblclick={(e) => {
                          e.stopPropagation()
                          startRename(s)
                        }}>{sessionTitle(s)}</span
                      >
                    {/if}
                    {#if s.permission === 'bypass'}<span class="perm" title={permHelp(s.permission)}>⚠</span>{/if}
                    {#if s.git && s.git.files > 0}
                      <span class="changes" title={changesTitle(s)}>{changesLabel(s.git)}</span>
                    {/if}
                    {#if s.msgPending}
                      <span class="mail" title={t('sidebar.msgPending', { n: s.msgPending })}
                        ><svg viewBox="0 0 12 9" width="11" height="9" aria-hidden="true"
                          ><rect x="0.6" y="0.6" width="10.8" height="7.8" rx="1.3" fill="none" stroke="currentColor" stroke-width="1.2" /><path
                            d="M1 1.2 6 5l5-3.8"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.2"
                            stroke-linejoin="round"
                          /></svg
                        >{s.msgPending}</span
                      >
                    {/if}
                    <span class="grow"></span>
                    {#if showsState(s)}<span class="state {badgeClass(s)}">{stateLabel(s)}</span>{/if}
                    {#if s.state === 'suspended'}
                      <button
                        class="resume"
                        title={t(`sidebar.resumeTitle.${s.resumeMode === 'agent' || s.resumeMode === 'args' ? s.resumeMode : 'fresh'}`)}
                        onclick={(e) => {
                          e.stopPropagation()
                          onResume(s.id)
                        }}>▶ {t('sidebar.resume')}</button
                      >
                      <button
                        class="icon kill"
                        title={t('sidebar.discard')}
                        onclick={(e) => {
                          e.stopPropagation()
                          onDiscard(s.id)
                        }}>×</button
                      >
                    {:else}
                      {#if i < 9 && p.id === currentProjectId && s.id === selectedId}<span class="kbd">⌘{i + 1}</span>{/if}
                      <button
                        class="icon kill"
                        title={s.state === 'exited' ? t('sidebar.removeFromList') : t('sidebar.stopSession')}
                        onclick={(e) => {
                          e.stopPropagation()
                          onKill(s.id)
                        }}>×</button
                      >
                    {/if}
                  </div>
                  {#if head || timer.text || s.model || s.port || s.browserUrl}
                    <div class="sub">
                      {#if s.mode === 'worktree' && s.branch}
                        <span class="chip" title={branchTitle(s)}>{@render branchText(s.git?.head || s.branch)}</span>
                      {:else if s.git?.head}
                        <span class="chip same" title={t('sidebar.folderBranch', { branch: s.git.head })}>{@render branchText(s.git.head)}</span>
                      {/if}
                      {#if s.model}<span class="model" title={t('sidebar.model', { model: s.model })}>{head ? '· ' : ''}{prettyModel(s.model)}</span>{/if}
                      <span class="grow"></span>
                      {#if s.port}
                        <button
                          class="port"
                          title={t('sidebar.port', { port: s.port })}
                          onclick={(e) => {
                            e.stopPropagation()
                            void ClipboardSetText(`localhost:${s.port}`)
                          }}>:{s.port}</button
                        >
                      {/if}
                      {#if s.browserUrl}
                        <button
                          class="browser"
                          title={t('sidebar.browser', { url: s.browserUrl })}
                          onclick={(e) => {
                            e.stopPropagation()
                            if (e.altKey) void api.closeBrowser(s.id)
                            else void ClipboardSetText(s.browserUrl ?? '')
                          }}>🌐</button
                        >
                      {/if}
                      {#if timer.text}<span class="timer" class:stalled={timer.stalled}>{timer.text}</span>{/if}
                    </div>
                  {/if}
                  {#if prog}<div class="row-progress"><ProgressBar progress={prog} /></div>{/if}
                </div>
              </li>
            {:else}
              <li class="no-sessions">{t('sidebar.noSessions')}</li>
            {/each}
          </ul>
          {@const orphans = orphansOf(p.id)}
          {#if orphans.length}
            <div class="orphans">
              <div class="orphans-title" title={t('sidebar.orphansTitle')}>
                {t('sidebar.orphans')}
              </div>
              {#each orphans as w (w.id)}
                <div class="orphan" title={t('sidebar.orphanTitle', { path: w.path, branch: w.branch, base: w.base })}>
                  <div class="line">
                    <span class="chip" title={w.branch}>{@render branchText(w.branch)}</span>
                    {#if w.git}<span class="changes">{changesLabel(w.git)}{w.git.ahead ? ` ↑${w.git.ahead}` : ''}</span>{/if}
                  </div>
                  <div class="orphan-actions">
                    <button onclick={() => onOpenWorktree(w.id)}>{t('sidebar.viewChanges')}</button>
                    <button onclick={() => reopen(p.id, w)}>{t('sidebar.openShell')}</button>
                    <button class="danger" onclick={() => onOpenWorktree(w.id, true)}>{t('sidebar.remove')}</button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      </section>
    {/each}
  </div>

  <div class="footer">
    <button class="add-project" onclick={onAddProject}>{t('sidebar.addProject')}</button>
    <button class="gear" title={t('sidebar.settings')} aria-label={t('sidebar.settingsLabel')} onclick={onOpenSettings}>⚙</button>
  </div>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="resizer"
    title={t('sidebar.resizeHint')}
    onpointerdown={resizeStart}
    onpointermove={resizeMove}
    onpointerup={resizeEnd}
    onpointercancel={resizeEnd}
    ondblclick={resizeReset}
  ></div>
</aside>

<style>
  .sidebar {
    position: relative;
    display: flex;
    flex-direction: column;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border);
    user-select: none;
  }
  .titlebar {
    height: 38px;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    padding: 0 12px 0 80px; /* room for the traffic lights */
  }
  .brand {
    font-size: 11px;
    letter-spacing: 0.08em;
    color: var(--muted);
    text-transform: lowercase;
  }
  /* The overview row: lined up with the project rows below (its icon in
   * the chevrons' column). */
  .nav {
    flex: none;
    padding: 2px 6px 6px;
    border-bottom: 1px solid var(--border);
  }
  .overview {
    display: flex;
    align-items: center;
    gap: 4px;
    width: 100%;
    height: 28px;
    padding: 0 6px 0 4px;
    border-radius: 6px;
    color: var(--fg-dim);
    text-align: left;
  }
  .overview:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .overview.active {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .overview-icon {
    width: 16px;
    display: flex;
    justify-content: center;
    color: var(--muted);
  }
  .overview:hover .overview-icon,
  .overview.active .overview-icon {
    color: var(--fg);
  }
  .overview-label {
    flex: 1;
    font-weight: 500;
  }
  .overview kbd {
    font: inherit;
    font-size: 11px;
    color: var(--muted);
  }
  .projects {
    flex: 1;
    overflow-y: auto;
    padding: 4px 6px 8px;
  }
  .project + .project {
    margin-top: 4px;
  }
  .project-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
    height: 28px;
    padding: 0 4px;
    border-radius: 6px;
    cursor: default;
    color: var(--fg-dim);
    font-weight: 600;
    font-size: 12px;
  }
  .project-row:hover {
    background: var(--hover);
  }
  /* Drag & drop: the dragged row fades and a line marks where it goes. */
  .dragging {
    opacity: 0.4;
  }
  .drop-before::before,
  .drop-after::after {
    content: '';
    position: absolute;
    left: 4px;
    right: 4px;
    height: 2px;
    border-radius: 1px;
    background: var(--blue);
    pointer-events: none;
  }
  .drop-before::before {
    top: -1px;
  }
  .drop-after::after {
    bottom: -1px;
  }
  .project.current > .project-row {
    color: var(--fg);
  }
  .chevron {
    width: 16px;
    color: var(--muted);
    font-size: 10px;
    transition: transform 0.12s;
  }
  .chevron.closed {
    transform: rotate(-90deg);
  }
  .badge-btn {
    display: flex;
    padding: 0;
    border-radius: 4px;
  }
  .badge-btn:focus-visible {
    outline: 2px solid var(--blue);
    outline-offset: 1px;
  }
  .project-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .count {
    color: var(--muted);
    font-weight: 400;
    font-size: 11px;
  }
  .row-actions {
    display: flex;
    gap: 2px;
  }
  .row-actions .icon:not(:last-child) {
    visibility: hidden;
  }
  .project-row:hover .row-actions .icon:not(:last-child) {
    visibility: visible;
  }
  .icon {
    width: 20px;
    height: 20px;
    border-radius: 4px;
    color: var(--muted);
    font-size: 15px;
    line-height: 1;
  }
  .icon :global(svg) {
    margin: auto;
  }
  .icon:hover {
    background: var(--hover-strong);
    color: var(--fg);
  }
  .menu {
    position: absolute;
    top: 28px;
    right: 4px;
    z-index: 10;
    min-width: 180px;
    padding: 4px;
    background: var(--menu-bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    box-shadow: 0 8px 24px var(--shadow);
    font-weight: 400;
  }
  .menu-item {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    padding: 6px 8px;
    border-radius: 5px;
    color: var(--fg);
    text-align: left;
  }
  .menu-item:hover {
    background: var(--accent-bg);
  }
  .menu-cmd {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11px;
  }
  .menu-empty {
    padding: 6px 8px;
    color: var(--muted);
  }
  .sessions {
    list-style: none;
    margin: 0;
    padding: 0 0 0 10px;
  }
  .session {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: center;
    min-height: 26px;
    padding: 2px 4px 2px 8px;
    border-radius: 6px;
    color: var(--fg-dim);
    cursor: default;
  }
  .line {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 22px;
  }
  .sub {
    display: flex;
    align-items: center;
    gap: 5px;
    margin: -2px 0 2px 14px;
    font-size: 10.5px;
    color: var(--muted);
    min-width: 0;
  }
  /* Pushes what follows to the row's right edge. */
  .grow {
    flex: 1;
    min-width: 0;
  }
  .timer {
    flex: none;
    font-variant-numeric: tabular-nums;
  }
  .timer.stalled {
    color: var(--amber);
    font-weight: 600;
  }
  .row-progress {
    margin: 0 0 2px 14px;
  }
  .session:hover {
    background: var(--hover);
  }
  .session.selected {
    background: var(--accent-bg);
    color: var(--fg);
  }
  /* Shown in another split pane. */
  .session.shown {
    box-shadow: inset 2px 0 0 color-mix(in srgb, var(--blue) 60%, transparent);
    color: var(--fg);
  }
  .rename {
    flex: 0 1 60%;
    min-width: 0;
    padding: 0 4px;
    font: inherit;
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--blue);
    border-radius: 4px;
    outline: none;
  }
  /* Second line: the branch chip takes the room and shrinks first; the
     model and the timer keep theirs. */
  .model {
    flex: none;
    max-width: 40%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 10.5px;
    color: var(--muted);
  }
  .session-name {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    flex: none;
    font-size: 10.5px;
    color: var(--muted);
  }
  .state.waiting_input,
  .state.attention {
    color: var(--amber);
    font-weight: 600;
  }
  .state.attention.done {
    color: var(--green);
  }
  .state.attention.error {
    color: var(--red);
  }
  .session.needs {
    color: var(--fg);
    background: color-mix(in srgb, var(--amber) 9%, transparent);
    box-shadow: inset 2px 0 0 var(--amber);
  }
  .session.needs.selected {
    background: var(--accent-bg);
  }
  .attn-count {
    min-width: 16px;
    height: 16px;
    padding: 0 5px;
    border-radius: 8px;
    background: var(--amber);
    color: #1d1f24;
    font-size: 10.5px;
    font-weight: 700;
    line-height: 16px;
    text-align: center;
  }
  .state.exited.fail {
    color: var(--red);
  }
  .restored {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin: 6px 8px 2px;
    padding: 6px 8px;
    border-radius: 6px;
    background: color-mix(in srgb, var(--amber) 12%, transparent);
    font-size: 11.5px;
  }
  .restored-actions {
    display: flex;
    gap: 6px;
  }
  .restored button,
  .session .resume {
    font-size: 10.5px;
    padding: 1px 6px;
  }
  .restored .danger {
    color: var(--red);
  }
  .dot.suspended {
    background: transparent;
    box-shadow: inset 0 0 0 1.5px var(--grey);
  }
  .kbd {
    font-size: 10px;
    color: var(--muted);
    opacity: 0.7;
  }
  .session .kill {
    display: none;
  }
  .session:hover .kill {
    display: inline-block;
  }
  .session:hover .kbd {
    display: none;
  }
  .mail {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
  }
  .dot {
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--grey);
  }
  .dot.running {
    background: var(--blue);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .dot.waiting_input {
    background: var(--amber);
    box-shadow: 0 0 6px var(--amber);
  }
  .dot.idle {
    background: var(--green);
  }
  .dot.attention {
    background: var(--amber);
    animation: attn 1.4s ease-in-out infinite;
  }
  .dot.attention.done {
    background: var(--green);
    animation: none;
    box-shadow: 0 0 6px var(--green);
  }
  .dot.attention.error {
    background: var(--red);
    animation: none;
    box-shadow: 0 0 6px var(--red);
  }
  @keyframes attn {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--amber) 70%, transparent);
    }
    50% {
      box-shadow: 0 0 0 4px color-mix(in srgb, var(--amber) 0%, transparent);
    }
  }
  .dot.exited {
    background: var(--grey);
  }
  .dot.exited.fail {
    background: var(--red);
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
      box-shadow: 0 0 0 0 rgba(97, 175, 239, 0.6);
    }
    50% {
      opacity: 0.55;
      box-shadow: 0 0 0 3px rgba(97, 175, 239, 0);
    }
  }
  .no-sessions {
    padding: 4px 8px 6px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .footer {
    flex: none;
    display: flex;
    gap: 6px;
    padding: 8px;
    border-top: 1px solid var(--border);
  }
  .gear {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 6px;
    color: var(--muted);
    font-size: 15px;
  }
  .gear:hover {
    color: var(--fg);
    background: var(--hover);
  }
  /* As wide as the project row: wider would spill past the sidebar's
     left edge (and be clipped by its scroller). */
  .menu.wide {
    left: 0;
    right: 0;
    min-width: 0;
  }
  .spawn-form {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 2px;
  }
  .spawn-head {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 2px 6px;
    color: var(--fg);
  }
  .back {
    font-size: 16px;
  }
  .opt {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 5px 6px;
    border-radius: 5px;
    color: var(--fg);
  }
  .opt:hover:not(.disabled) {
    background: var(--hover);
  }
  .opt.disabled {
    opacity: 0.5;
  }
  .opt input {
    margin-top: 2px;
    accent-color: var(--blue);
  }
  .opt span {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .opt b {
    font-weight: 500;
  }
  .opt em {
    font-style: normal;
    font-size: 11px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 200px;
  }
  .shared-hint {
    margin: 2px 6px 4px 28px;
    font-size: 11px;
    line-height: 1.35;
    color: var(--muted);
    max-width: 260px;
  }
  .shared-hint.warn {
    color: var(--amber);
  }
  .fld {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 3px 6px 3px 28px;
    font-size: 11.5px;
    color: var(--fg-dim);
  }
  .fld span {
    width: 34px;
  }
  .fld input,
  .fld select {
    flex: 1;
    min-width: 0;
    font: 11.5px var(--mono);
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    padding: 3px 5px;
  }
  .spawn-actions {
    display: flex;
    justify-content: flex-end;
    padding: 6px 2px 2px;
  }
  .launch {
    padding: 4px 12px;
    border-radius: 5px;
    background: var(--accent-bg);
    color: var(--fg);
    border: 1px solid var(--border-strong);
  }
  .launch:hover {
    background: var(--hover-strong);
  }
  .chip {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding: 0 5px;
    border-radius: 8px;
    font: 10px var(--mono);
    line-height: 15px;
    color: var(--blue);
    background: color-mix(in srgb, var(--blue) 12%, transparent);
  }
  /* The project folder's own branch (not a worktree of the session's). */
  .chip {
    display: inline-flex;
  }
  .chip .bh {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chip .bt {
    flex: none;
  }
  .chip.same {
    color: var(--fg-dim);
    background: color-mix(in srgb, var(--fg-dim) 10%, transparent);
  }
  .port {
    flex: none;
    padding: 0 4px;
    border-radius: 6px;
    font: 10px var(--mono);
    color: var(--teal);
    background: color-mix(in srgb, var(--teal) 12%, transparent);
  }
  .port:hover {
    background: color-mix(in srgb, var(--teal) 24%, transparent);
  }
  .browser {
    flex: none;
    padding: 0 3px;
    border-radius: 6px;
    font-size: 10px;
    line-height: 1.4;
  }
  .browser:hover {
    background: color-mix(in srgb, var(--blue) 18%, transparent);
  }
  .perm {
    flex: none;
    font-size: 10px;
    color: var(--amber);
  }
  .changes {
    flex: none;
    font: 10px var(--mono);
    color: var(--amber);
  }
  .orphans {
    margin: 2px 0 4px 10px;
    padding: 4px 0 2px;
    border-top: 1px dashed var(--border-strong);
  }
  .orphans-title {
    padding: 0 8px 2px;
    font-size: 10px;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .orphan {
    padding: 2px 4px 4px 8px;
    border-radius: 6px;
  }
  .orphan:hover {
    background: var(--hover);
  }
  .orphan .line {
    height: 20px;
  }
  .orphan .chip {
    max-width: 150px;
  }
  .orphan-actions {
    display: flex;
    gap: 8px;
    font-size: 10.5px;
  }
  .orphan-actions button {
    color: var(--muted);
  }
  .orphan-actions button:hover {
    color: var(--fg);
    text-decoration: underline;
  }
  .orphan-actions .danger:hover {
    color: var(--red);
  }
  .menu-sep {
    height: 1px;
    margin: 4px 2px;
    background: var(--border-strong);
  }
  .menu-item.muted {
    color: var(--fg-dim);
  }
  .add-project {
    flex: 1;
    height: 28px;
    border-radius: 6px;
    color: var(--fg-dim);
    border: 1px dashed var(--border-strong);
  }
  .add-project:hover {
    color: var(--fg);
    background: var(--hover);
  }
  /* The right edge, dragged to resize the sidebar. */
  .resizer {
    position: absolute;
    top: 0;
    right: -4px;
    bottom: 0;
    width: 8px;
    cursor: col-resize;
    z-index: 5;
  }
</style>
