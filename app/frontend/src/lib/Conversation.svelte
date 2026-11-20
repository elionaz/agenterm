<script lang="ts">
  // A project's conversation: every message between its sessions (and from
  // the user), in order, each side in its session's color, with whether it
  // is held for the user, on its way or delivered. In manual projects the
  // user delivers held messages from here.
  import { api, errorMessage, type Project, type Session } from './api'
  import { locale, t } from './i18n.svelte'
  import { conversationRows, heldRecipients, isShellSession, USER, type Message } from './messages'
  import { messagesAuto, sessionColor } from './msgactivity'

  interface Props {
    project: Project
    sessions: Session[]
    defaultAuto: boolean
    /** Bumped on message.changed / message.sent in this project: reload. */
    stamp: number
    onUpdateProject: (patch: { messages: string }) => Promise<void>
    onSelect: (sessionId: string) => void
    onClose: () => void
  }

  let { project, sessions, defaultAuto, stamp, onUpdateProject, onSelect, onClose }: Props = $props()

  let messages = $state<Message[]>([])
  let error = $state('')
  let only = $state('')
  let to = $state('all')
  let draft = $state('')
  let busy = $state(false)
  let listEl = $state<HTMLDivElement>()
  let seq = 0

  const live = $derived(sessions.filter((s) => s.projectId === project.id && s.state !== 'exited'))
  const auto = $derived(messagesAuto(project, defaultAuto))
  const rows = $derived(conversationRows(messages, sessions, only))
  const held = $derived(heldRecipients(conversationRows(messages, sessions)))
  /** Everyone that appears in the conversation or is open, for the filter chips. */
  const people = $derived.by(() => {
    const ids: string[] = []
    for (const s of live) ids.push(s.id)
    for (const m of messages) for (const id of [m.from, m.to]) if (id !== USER && !ids.includes(id)) ids.push(id)
    return ids
  })

  function nameOf(id: string): string {
    if (id === USER) return t('msg.you')
    const s = sessions.find((x) => x.id === id)
    if (s) return s.name || s.profileId
    const m = messages.find((x) => x.from === id || x.to === id)
    return (m && (m.from === id ? m.fromName : m.toName)) || id
  }

  async function load() {
    const my = ++seq
    try {
      const ms = await api.listProjectMessages(project.id)
      if (my !== seq) return
      const atBottom = !listEl || listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 40
      messages = ms
      error = ''
      if (atBottom) queueMicrotask(() => listEl?.scrollTo({ top: listEl.scrollHeight }))
    } catch (e) {
      if (my === seq) error = errorMessage(e)
    }
  }

  $effect(() => {
    void stamp
    void project.id
    void load()
  })

  async function run(fn: () => Promise<unknown>) {
    busy = true
    try {
      await fn()
      error = ''
    } catch (e) {
      error = errorMessage(e)
    } finally {
      busy = false
    }
  }

  function send() {
    const body = draft.trim()
    if (!body || busy) return
    void run(async () => {
      if (to === 'all') await api.sendMessageAll(project.id, body)
      else await api.sendMessage(to, body)
      draft = ''
      await load()
    })
  }

  function when(iso: string): string {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''
    const today = d.toDateString() === new Date().toDateString()
    return d.toLocaleString(locale(), today ? { hour: '2-digit', minute: '2-digit' } : { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }

  function colorVar(id: string): string {
    const c = sessionColor(id)
    return c ? `var(--${c})` : 'var(--fg-dim)'
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="convo">
  <header class="titlebar drag">
    <span class="title">{t('convo.title', { project: project.name })}</span>
    <div class="mode" role="group" aria-label={t('convo.mode')}>
      <button class:active={auto} title={t('sidebar.msgAutoTitle')} onclick={() => void run(() => onUpdateProject({ messages: 'auto' }))}
        >{t('sidebar.msgAuto')}</button
      >
      <button class:active={!auto} title={t('sidebar.msgManualTitle')} onclick={() => void run(() => onUpdateProject({ messages: 'manual' }))}
        >{t('sidebar.msgManual')}</button
      >
    </div>
    {#if held.length}
      <button class="btn primary" disabled={busy} onclick={() => void run(async () => (await api.releaseMessages(project.id), await load()))}
        >{t('convo.deliverAll', { n: held.length })}</button
      >
    {/if}
    <button class="close" title={t('history.close')} onclick={onClose}>×</button>
  </header>

  <nav class="people">
    <button class:active={!only} onclick={() => (only = '')}>{t('convo.everyone')}</button>
    {#each people as id (id)}
      <button class:active={only === id} style:--c={colorVar(id)} onclick={() => (only = only === id ? '' : id)}
        ><span class="dot"></span>{nameOf(id)}</button
      >
    {/each}
  </nav>

  <div class="list" bind:this={listEl}>
    {#if error}<p class="note error">{error}</p>{/if}
    {#each rows as r (r.id)}
      <div class="msg" class:mine={r.from === USER} style:--c={colorVar(r.from)}>
        <div class="head">
          <button class="who" style:--c={colorVar(r.from)} onclick={() => r.from !== USER && onSelect(r.from)}
            >{r.from === USER ? t('msg.you') : r.fromName}</button
          >
          <span class="arrow">→</span>
          <button class="who" style:--c={colorVar(r.to)} onclick={() => onSelect(r.to)}>{r.toName}</button>
          <span class="when">{when(r.at)}</span>
          <span class="status {r.status}">{t(`convo.status.${r.status}`)}</span>
          {#if r.status === 'held'}
            {@const s = sessions.find((x) => x.id === r.to)}
            <button
              class="deliver"
              disabled={busy || !s}
              title={s && isShellSession(s) ? t('convo.deliverShell') : ''}
              onclick={() => void run(async () => (await api.deliverMessages(r.to), await load()))}>{t('msg.deliverNow')}</button
            >
          {/if}
        </div>
        <div class="body">{r.body}</div>
      </div>
    {:else}
      <p class="note">{t('convo.empty')}</p>
    {/each}
  </div>

  <footer class="compose">
    <select bind:value={to} aria-label={t('convo.to')}>
      <option value="all">{t('convo.toAll')}</option>
      {#each live as s (s.id)}<option value={s.id}>{s.name || s.profileId}</option>{/each}
    </select>
    <textarea
      bind:value={draft}
      rows="2"
      placeholder={t('convo.placeholder')}
      onkeydown={(e) => {
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
          e.preventDefault()
          send()
        }
      }}
    ></textarea>
    <button class="btn primary" disabled={busy || !draft.trim()} onclick={send}>{t('convo.send')}</button>
  </footer>
</div>

<style>
  .convo {
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
    gap: 12px;
    padding: 0 8px 0 14px;
    border-bottom: 1px solid var(--border);
  }
  .title {
    font-weight: 600;
  }
  .mode {
    display: flex;
    gap: 2px;
  }
  .mode button {
    padding: 2px 10px;
    border-radius: 5px;
    color: var(--fg-dim);
  }
  .mode button:hover {
    background: var(--hover);
  }
  .mode button.active {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .close {
    margin-left: auto;
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
  .people {
    flex: none;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    padding: 6px 14px;
    border-bottom: 1px solid var(--border);
  }
  .people button {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 2px 9px;
    border-radius: 10px;
    font-size: 11.5px;
    color: var(--fg-dim);
    border: 1px solid var(--border);
  }
  .people button.active {
    color: var(--fg);
    border-color: var(--c, var(--accent));
    background: color-mix(in srgb, var(--c, var(--accent)) 14%, transparent);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--c);
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 10px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .msg {
    max-width: 760px;
    padding: 7px 10px;
    border-radius: 8px;
    border-left: 3px solid var(--c);
    background: color-mix(in srgb, var(--c) 7%, transparent);
  }
  .msg.mine {
    border-left-color: var(--muted);
    background: var(--hover);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 6px;
    font-size: 11.5px;
  }
  .who {
    font-weight: 600;
    color: var(--c);
  }
  .who:hover {
    text-decoration: underline;
  }
  .arrow,
  .when {
    color: var(--muted);
  }
  .status {
    font-size: 10.5px;
    padding: 0 6px;
    border-radius: 8px;
    border: 1px solid var(--border);
    color: var(--muted);
  }
  .status.held {
    color: var(--amber);
    border-color: color-mix(in srgb, var(--amber) 50%, transparent);
  }
  .status.pending {
    color: var(--blue);
  }
  .deliver {
    font-size: 11px;
    color: var(--amber);
  }
  .deliver:hover {
    text-decoration: underline;
  }
  .body {
    margin-top: 3px;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 12.5px;
  }
  .compose {
    flex: none;
    display: flex;
    gap: 8px;
    align-items: flex-end;
    padding: 8px 14px 12px;
    border-top: 1px solid var(--border);
  }
  .compose select {
    flex: none;
    max-width: 180px;
  }
  .compose textarea {
    flex: 1;
    resize: none;
    padding: 6px 8px;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    background: transparent;
    color: var(--fg);
    font: inherit;
  }
  .btn {
    padding: 4px 12px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
  }
  .btn.primary {
    background: var(--accent-bg);
  }
  .btn:disabled {
    opacity: 0.5;
  }
  .note {
    color: var(--muted);
    font-size: 12px;
  }
  .note.error {
    color: var(--red);
  }
</style>
