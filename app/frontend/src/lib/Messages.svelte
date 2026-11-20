<script lang="ts">
  // The Messages tab: what a session sent and received from the other
  // sessions of its project (panal msg), the Deliver button, and a box
  // to write to it.
  import { api, errorMessage, type Session } from './api'
  import { lang, t, th } from './i18n.svelte'
  import { canDeliver, holdKey, isShellSession, messageRows, pendingFor, USER, type Message } from './messages'

  let {
    session,
    sessions,
    projectName,
    stamp = 0,
    onError,
  }: {
    session: Session
    sessions: Session[]
    projectName: string
    /** Bumped on message.changed for this session: reload. */
    stamp?: number
    onError: (msg: string) => void
  } = $props()

  let messages = $state<Message[]>([])
  let loaded = $state(false)
  let draft = $state('')
  let sending = $state(false)
  let delivering = $state(false)
  let listEl = $state<HTMLDivElement>()
  let seq = 0

  // The session object changes on every activity event: reload on its id.
  const sessionId = $derived(session.id)
  const rows = $derived(messageRows(messages, session.id, sessions))
  const pending = $derived(pendingFor(messages, session.id))
  const shell = $derived(isShellSession(session))

  async function load(id: string) {
    const my = ++seq
    try {
      const ms = await api.listMessages(id)
      if (my !== seq) return
      messages = ms
      loaded = true
      queueMicrotask(() => listEl?.scrollTo({ top: listEl.scrollHeight }))
    } catch (e) {
      if (my === seq) onError(errorMessage(e))
    }
  }

  $effect(() => {
    void stamp
    void load(sessionId)
  })

  async function send() {
    const body = draft.trim()
    if (!body || sending) return
    sending = true
    try {
      await api.sendMessage(session.id, body)
      draft = ''
      await load(session.id)
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      sending = false
    }
  }

  async function deliver() {
    delivering = true
    try {
      await api.deliverMessages(session.id)
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      delivering = false
    }
  }

  function when(at: string): string {
    const d = new Date(at)
    if (isNaN(d.getTime())) return ''
    const today = new Date().toDateString() === d.toDateString()
    return d.toLocaleString(lang(), today ? { hour: '2-digit', minute: '2-digit' } : { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }

  function who(r: (typeof rows)[number]): string {
    const name = r.peer === USER ? t('msg.you') : r.peerName
    return t(r.dir === 'in' ? 'msg.from' : 'msg.to', { name })
  }
</script>

<div class="messages">
  <p class="hint">{@html th('msg.hint', { project: projectName })}</p>

  {#if pending > 0}
    <div class="waiting">
      <span class="count">{t('msg.waiting', { n: pending })}</span>
      {#if shell}
        <span class="help">{t('msg.shellHelp')}</span>
      {:else}
        <span class="help">{t(holdKey(session.msgHold, session.state))}</span>
        {#if canDeliver(session, pending, session.msgHold === 'held')}
          <button class="btn" disabled={delivering} onclick={deliver} title={t('msg.deliverHelp')}
            >{session.msgHold === 'nudged' ? t('msg.remind') : t('msg.deliverNow')}</button
          >
        {/if}
      {/if}
    </div>
  {/if}

  <div class="list" bind:this={listEl}>
    {#if loaded && rows.length === 0}
      <p class="empty">{@html th('msg.empty')}</p>
    {/if}
    {#each rows as r (r.id)}
      <article class="msg {r.dir}" class:pending={r.pending}>
        <header>
          <span class="who">{who(r)}</span>
          {#if r.gone}<span class="tag">{t('msg.gone')}</span>{/if}
          <span class="at">{when(r.at)}</span>
          <span class="state">{r.pending ? t('msg.pending') : t('msg.delivered')}</span>
        </header>
        <div class="body">{r.body}</div>
      </article>
    {/each}
  </div>

  <form
    class="compose"
    onsubmit={(e) => {
      e.preventDefault()
      void send()
    }}
  >
    <textarea
      bind:value={draft}
      rows="2"
      placeholder={t('msg.compose')}
      spellcheck="false"
      onkeydown={(e) => {
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
          e.preventDefault()
          void send()
        }
      }}
    ></textarea>
    <div class="compose-row">
      <span class="help">{t('msg.sendHelp')}</span>
      <button class="btn primary" type="submit" disabled={sending || !draft.trim()}>{t('msg.send')}</button>
    </div>
  </form>
</div>

<style>
  .messages {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: flex;
    flex-direction: column;
    background: var(--bg);
    color: var(--fg);
    font-size: 13px;
  }
  .hint {
    flex: none;
    margin: 0;
    padding: 8px 14px;
    color: var(--muted);
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .hint :global(code),
  .empty :global(code) {
    font-family: var(--mono);
    font-size: 11.5px;
  }
  .waiting {
    flex: none;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px 10px;
    padding: 8px 14px;
    background: color-mix(in srgb, var(--amber) 12%, transparent);
    border-bottom: 1px solid var(--border);
  }
  .count {
    font-weight: 600;
  }
  .help {
    color: var(--muted);
    font-size: 12px;
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
  .empty {
    margin: auto;
    max-width: 420px;
    text-align: center;
    color: var(--muted);
    line-height: 1.5;
  }
  .msg {
    max-width: min(640px, 85%);
    padding: 7px 10px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--sidebar-bg);
  }
  .msg.out {
    align-self: flex-end;
    background: var(--accent-bg);
  }
  .msg.pending {
    border-color: color-mix(in srgb, var(--amber) 55%, transparent);
  }
  .msg header {
    display: flex;
    gap: 8px;
    align-items: baseline;
    font-size: 11.5px;
    color: var(--fg-dim);
    margin-bottom: 3px;
  }
  .who {
    font-weight: 600;
    color: var(--fg);
  }
  .tag {
    color: var(--muted);
    font-style: italic;
  }
  .at {
    margin-left: auto;
    color: var(--muted);
  }
  .state {
    color: var(--muted);
  }
  .msg.pending .state {
    color: var(--amber);
  }
  .body {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.45;
  }
  .compose {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 14px 10px;
    border-top: 1px solid var(--border);
  }
  textarea {
    resize: vertical;
    min-height: 44px;
    max-height: 200px;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
    background: var(--bg);
    color: var(--fg);
    font: inherit;
  }
  .compose-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .compose-row .btn {
    margin-left: auto;
  }
  .btn {
    padding: 3px 9px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
    font-size: 12px;
    color: var(--fg-dim);
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
    color: var(--fg);
  }
  .btn:disabled {
    opacity: 0.45;
  }
  .btn.primary {
    background: var(--accent-bg);
    color: var(--fg);
  }
</style>
