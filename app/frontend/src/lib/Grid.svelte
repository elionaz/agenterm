<script lang="ts">
  import { prettyModel } from './models'
  // Overview of every session as a card: status, activity, timers, progress
  // and the last lines of output (plain text from session.tail — no xterm).
  import { onDestroy, onMount } from 'svelte'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { api, changesLabel, needsAttention, sessionTitle, type DaemonEvent, type Project, type Session } from './api'
  import { activityText, progressOf, stateLabel, timerInfo } from './activity'
  import { t } from './i18n.svelte'
  import ProgressBar from './ProgressBar.svelte'
  import ProjectBadge from './ProjectBadge.svelte'

  interface Props {
    projects: Project[]
    sessions: Session[]
    /** Project shown when "all projects" is off. */
    projectId: string | null
    selectedId: string | null
    now: number
    onOpen: (id: string) => void
    onClose: () => void
  }

  let { projects, sessions, projectId, selectedId, now, onOpen, onClose }: Props = $props()

  const TAIL_LINES = 8
  /** Minimum time between two tail fetches of the same session. */
  const TAIL_EVERY = 1500

  let allProjects = $state(false)
  let tails = $state<Record<string, string[]>>({})

  const projectName = (id: string) => projects.find((p) => p.id === id)?.name ?? id
  const shown = $derived(
    allProjects || !projectId ? sessions : sessions.filter((s) => s.projectId === projectId),
  )
  const title = $derived(allProjects || !projectId ? t('grid.allSessions') : projectName(projectId))

  // Tail refresh: sessions with news are marked dirty and fetched at most
  // every TAIL_EVERY ms.
  const dirty = new Set<string>()
  const fetchedAt = new Map<string, number>()
  const inflight = new Set<string>()

  async function fetchTail(id: string) {
    if (inflight.has(id)) return
    inflight.add(id)
    dirty.delete(id)
    fetchedAt.set(id, Date.now())
    try {
      const lines = await api.tail(id, TAIL_LINES)
      tails[id] = lines
    } catch {
      /* session gone */
    } finally {
      inflight.delete(id)
    }
  }

  function pump() {
    const ids = new Set(shown.map((s) => s.id))
    const t = Date.now()
    for (const id of ids) {
      if (!(id in tails) || (dirty.has(id) && t - (fetchedAt.get(id) ?? 0) >= TAIL_EVERY)) void fetchTail(id)
    }
  }

  function onEvent(ev: DaemonEvent) {
    if (ev.type === 'session.activity' || ev.type === 'session.state') dirty.add(ev.sessionId)
  }

  function badgeClass(s: Session): string {
    if (s.state === 'exited') return s.exitCode ? 'exited fail' : 'exited'
    if (needsAttention(s)) return `attention ${s.attentionKind}`
    if (s.stalled && s.state === 'running') return 'stalled'
    return s.state
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  let timer: number | undefined
  let off: (() => void) | undefined
  onMount(() => {
    off = EventsOn('daemon:event', onEvent)
    pump()
    timer = window.setInterval(pump, 500)
  })
  onDestroy(() => {
    off?.()
    clearInterval(timer)
  })
</script>

<svelte:window onkeydown={onKeydown} />

<div class="grid-view">
  <header class="head">
    <span class="title">{title}</span>
    <span class="count">{t('grid.count', { n: shown.length })}</span>
    <label class="toggle">
      <input type="checkbox" bind:checked={allProjects} />
      {t('grid.allProjects')}
    </label>
    <button class="close" title={t('grid.close')} onclick={onClose}>×</button>
  </header>

  {#if shown.length === 0}
    <div class="empty">{allProjects || !projectId ? t('grid.empty') : t('grid.emptyProject')}</div>
  {:else}
    <div class="cards">
      {#each shown as s (s.id)}
        {@const timer = timerInfo(s, now, true)}
        {@const prog = s.state !== 'exited' ? progressOf(s) : undefined}
        <button
          class="card {badgeClass(s)}"
          class:selected={s.id === selectedId}
          title={activityText(s)}
          onclick={() => onOpen(s.id)}
        >
          <div class="card-head">
            <span class="dot {badgeClass(s)}"></span>
            <span class="name">{sessionTitle(s)}</span>
            {#if s.model}<span class="model" title={s.model}>{prettyModel(s.model)}</span>{/if}
            {#if allProjects || !projectId}
              {@const proj = projects.find((p) => p.id === s.projectId)}
              <span class="proj">{#if proj}<ProjectBadge project={proj} size={13} />{/if}<span class="proj-name"
                  >{projectName(s.projectId)}</span
                ></span
              >
            {/if}
            <span class="badge {badgeClass(s)}">{stateLabel(s)}</span>
          </div>
          {#if (s.mode === 'worktree' && s.branch) || s.git?.head || (s.git && s.git.files > 0)}
            <div class="git">
              {#if s.mode === 'worktree' && s.branch}<span class="branch">⎇ {s.branch}</span>
              {:else if s.git?.head}<span class="branch same">⎇ {s.git.head}</span>{/if}
              {#if s.git && s.git.files > 0}<span class="changes">{changesLabel(s.git)}</span>{/if}
              {#if s.git?.ahead}<span class="ahead">↑{s.git.ahead}</span>{/if}
            </div>
          {/if}
          {#if s.attentionKind && s.state !== 'exited' && s.attentionMessage}
            <div class="attn {s.attentionKind}">{s.attentionMessage}</div>
          {/if}
          <div class="activity">{s.state !== 'exited' ? activityText(s) : ''}&nbsp;</div>
          <div class="timer" class:stalled={timer.stalled}>{timer.text}&nbsp;</div>
          {#if prog}<ProgressBar progress={prog} />{/if}
          <pre class="tail">{(tails[s.id] ?? []).join('\n')}</pre>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .git {
    display: flex;
    gap: 8px;
    min-width: 0;
    font: 11px var(--mono);
  }
  .git .branch {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--blue);
  }
  .git .branch.same {
    color: var(--fg-dim);
  }
  .model {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11px;
    color: var(--muted);
  }
  .git .changes {
    flex: none;
    color: var(--amber);
  }
  .git .ahead {
    flex: none;
    color: var(--muted);
  }
  .grid-view {
    position: absolute;
    inset: 38px 0 0 0;
    z-index: 5;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }
  .head {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
  }
  .title {
    font-weight: 600;
  }
  .count {
    color: var(--muted);
    font-size: 12px;
  }
  .toggle {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--fg-dim);
    font-size: 12px;
  }
  .toggle input {
    accent-color: var(--blue);
  }
  .close {
    width: 24px;
    height: 24px;
    border-radius: 5px;
    color: var(--muted);
    font-size: 16px;
  }
  .close:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .empty {
    padding: 40px;
    text-align: center;
    color: var(--muted);
  }
  .cards {
    flex: 1;
    overflow-y: auto;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 12px;
    padding: 14px 16px;
    align-content: start;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
    padding: 10px 12px;
    text-align: left;
    border-radius: 8px;
    border: 1px solid var(--border-strong);
    background: var(--sidebar-bg);
    color: var(--fg-dim);
  }
  .card:hover {
    border-color: color-mix(in srgb, var(--blue) 45%, var(--border-strong));
  }
  .card.selected {
    border-color: color-mix(in srgb, var(--blue) 70%, transparent);
  }
  .card.attention {
    box-shadow: inset 3px 0 0 var(--amber);
  }
  .card.attention.done {
    box-shadow: inset 3px 0 0 var(--green);
  }
  .card.attention.error {
    box-shadow: inset 3px 0 0 var(--red);
  }
  .card.stalled {
    box-shadow: inset 3px 0 0 var(--orange);
  }
  .card-head {
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    color: var(--fg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .proj {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    color: var(--muted);
    font-size: 11px;
  }
  .proj-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    margin-left: auto;
    flex: none;
    font-size: 10.5px;
    color: var(--muted);
  }
  .badge.attention,
  .badge.waiting_input {
    color: var(--amber);
    font-weight: 600;
  }
  .badge.attention.done {
    color: var(--green);
  }
  .badge.attention.error,
  .badge.exited.fail {
    color: var(--red);
  }
  .badge.running {
    color: var(--blue);
  }
  .badge.stalled {
    color: var(--orange);
    font-weight: 600;
  }
  .attn {
    font-size: 11.5px;
    color: var(--amber);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .attn.done {
    color: var(--green);
  }
  .attn.error {
    color: var(--red);
  }
  .activity {
    font-size: 12px;
    color: var(--fg-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .timer {
    font-size: 11px;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .timer.stalled {
    color: var(--orange);
    font-weight: 600;
  }
  .tail {
    margin: 4px 0 0;
    height: calc(8 * 1.35em);
    overflow: hidden;
    padding: 6px 8px;
    border-radius: 5px;
    background: var(--bg);
    color: var(--fg-dim);
    font-family: var(--mono);
    font-size: 10.5px;
    line-height: 1.35;
    white-space: pre;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
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
  }
  .dot.waiting_input,
  .dot.attention {
    background: var(--amber);
  }
  .dot.idle,
  .dot.attention.done {
    background: var(--green);
  }
  .dot.attention.error,
  .dot.exited.fail {
    background: var(--red);
  }
  .dot.stalled {
    background: var(--orange);
  }
</style>
