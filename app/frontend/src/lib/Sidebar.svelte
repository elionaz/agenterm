<script lang="ts">
  import {
    api,
    attentionLabel,
    changesLabel,
    needsAttention,
    rememberMode,
    rememberedMode,
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
  import ProgressBar from './ProgressBar.svelte'

  interface Props {
    projects: Project[]
    sessions: Session[]
    profiles: Profile[]
    selectedId: string | null
    currentProjectId: string | null
    onSelect: (id: string) => void
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
    onOpenSettings: () => void
    /** Opens the grid overview (focused on a project when given). */
    onOpenGrid: (projectId?: string) => void
    /** Current time (ms), ticking once per second, for the timers. */
    now: number
  }

  let {
    projects,
    sessions,
    profiles,
    selectedId,
    currentProjectId,
    onSelect,
    onSelectProject,
    onSpawn,
    onKill,
    onResume,
    onDiscard,
    onResumeAll,
    onDiscardAll,
    onAddProject,
    onRemoveProject,
    onOpenSettings,
    onOpenGrid,
    now,
    worktrees,
    onOpenWorktree,
  }: Props = $props()

  let collapsed = $state<Record<string, boolean>>({})
  let menuFor = $state<string | null>(null)
  /** Second step of the spawn menu: the profile picked, where to run it. */
  let picked = $state<Profile | null>(null)
  let projGit = $state<ProjectGit | null>(null)
  let mode = $state<SpawnMode>('same')
  let branch = $state('')
  let base = $state('')
  let defaultMode: SpawnMode = 'same'

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

  const suspendedCount = $derived(sessions.filter((s) => s.state === 'suspended').length)

  function tooltip(s: Session): string {
    let base = `${s.cmd} · ${s.cwd} · ${s.id}`
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
      .then((st) => (defaultMode = st.git?.defaultMode === 'worktree' ? 'worktree' : 'same'))
      .catch(() => {})
  }

  const canWorktree = $derived(!!projGit?.isRepo && !projGit.error)

  function randomSuffix(): string {
    return Math.random().toString(16).slice(2, 6)
  }

  function pick(projectId: string, prof: Profile) {
    picked = prof
    mode = rememberedMode(projectId, defaultMode)
    if (mode === 'worktree' && projGit && !canWorktree) mode = 'same'
    branch = `agent/${prof.id}-${randomSuffix()}`
    base = projGit?.defaultBase || projGit?.currentBranch || ''
  }

  function launch(projectId: string) {
    if (!picked) return
    const wt = mode === 'worktree' && canWorktree
    rememberMode(projectId, wt ? 'worktree' : 'same')
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
    let out = t('sidebar.worktreeOn', { branch: s.branch ?? '' })
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

<svelte:window onclick={closeMenu} onkeydown={(e) => e.key === 'Escape' && menuFor && closeMenu()} />

<aside class="sidebar">
  <div class="titlebar drag">
    <button class="icon grid-btn" title={t('sidebar.overview')} aria-label={t('sidebar.overviewLabel')} onclick={() => onOpenGrid()}>▦</button>
    <span class="brand">agenterm</span>
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
    {#each projects as p (p.id)}
      {@const list = byProject.get(p.id) ?? []}
      {@const pending = list.filter(needsAttention).length}
      <section class="project" class:current={p.id === currentProjectId}>
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="project-row" title={p.path} onclick={() => onSelectProject(p.id)}>
          <button
            class="chevron"
            class:closed={collapsed[p.id]}
            aria-label={collapsed[p.id] ? t('sidebar.expand') : t('sidebar.collapse')}
            onclick={(e) => {
              e.stopPropagation()
              collapsed[p.id] = !collapsed[p.id]
            }}>▾</button
          >
          <span class="project-name">{p.name}</span>
          {#if pending}
            <span class="attn-count" title={t('sidebar.needAttention', { n: pending })}
              >{pending}</span
            >
          {:else if list.length}<span class="count">{list.length}</span>{/if}
          <span class="row-actions">
            <button class="icon" title={t('sidebar.projectOverview')} onclick={(e) => {
              e.stopPropagation()
              onOpenGrid(p.id)
            }}>▦</button>
            <button class="icon" title={t('sidebar.removeProject')} onclick={(e) => {
              e.stopPropagation()
              onRemoveProject(p.id)
            }}>−</button>
            <button class="icon" title={t('sidebar.newSession')} onclick={(e) => toggleMenu(p.id, e)}>+</button>
          </span>
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
              <li>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="session"
                  class:selected={s.id === selectedId}
                  class:needs={needsAttention(s)}
                  title={tooltip(s)}
                  onclick={() => onSelect(s.id)}
                >
                  <div class="line">
                    <span class="dot {badgeClass(s)}"></span>
                    <span class="session-name">{s.profileId}</span>
                    {#if s.mode === 'worktree' && s.branch}
                      <span class="chip" title={branchTitle(s)}>⎇ {s.branch.replace(/^agent\//, '')}</span>
                    {/if}
                    {#if s.git && s.git.files > 0}
                      <span class="changes" title={t('sidebar.filesChanged', { n: s.git.files })}>{changesLabel(s.git)}</span>
                    {/if}
                    <span class="state {badgeClass(s)}">{stateLabel(s)}</span>
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
                      {#if i < 9 && p.id === currentProjectId}<span class="kbd">⌘{i + 1}</span>{/if}
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
                  {#if (s.activity && s.state !== 'exited') || timer.text}
                    <div class="sub">
                      <span class="activity">{s.state !== 'exited' ? activityText(s) : ''}</span>
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
                    <span class="chip">⎇ {w.branch.replace(/^agent\//, '')}</span>
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
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    width: 248px;
    min-width: 248px;
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
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
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
    align-items: baseline;
    gap: 6px;
    margin: -2px 0 2px 14px;
    font-size: 10.5px;
    color: var(--muted);
    min-width: 0;
  }
  .activity {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
  .grid-btn {
    margin-right: auto;
    font-size: 13px;
  }
  .session:hover {
    background: var(--hover);
  }
  .session.selected {
    background: var(--accent-bg);
    color: var(--fg);
  }
  /* The name keeps its width; the branch chip shrinks first (longer state
     words, e.g. "esperando", would otherwise squeeze the name to "s…"). */
  .session-name {
    flex: 0 0 auto;
    max-width: 50%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    margin-left: auto;
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
    flex: 0 100 auto;
    min-width: 0;
    max-width: 90px;
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
</style>
