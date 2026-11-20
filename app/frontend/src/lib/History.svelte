<script lang="ts">
  // Past sessions: search the ended agent sessions, read one (its prompts
  // and terminal text), reopen its conversation or delete it.
  import { tick } from 'svelte'
  import { api, errorMessage, type HistoryDetail, type HistoryHit, type Project } from './api'
  import { duration, highlight, matchingLines } from './historyview'
  import { locale, t } from './i18n.svelte'

  interface Props {
    projects: Project[]
    /** The project in view: the default scope. */
    projectId: string | null
    /** Bumped on history.changed (an archive or a delete). */
    stamp: number
    /** Reopens an archived session's conversation (rejects with a message). */
    onResume: (id: string) => Promise<void>
    onClose: () => void
  }

  let { projects, projectId, stamp, onResume, onClose }: Props = $props()

  let query = $state('')
  // The scope starts on the project in view (when there is one).
  let scope = $state<'project' | 'all'>('project')
  let hits = $state<HistoryHit[]>([])
  let loading = $state(false)
  let error = $state('')
  let picked = $state<string | null>(null)
  let detail = $state<HistoryDetail | null>(null)
  let detailError = $state('')
  let busy = $state(false)
  let confirmDelete = $state(false)
  let input = $state<HTMLInputElement>()
  let textBox = $state<HTMLPreElement>()

  const scopeId = $derived(scope === 'project' && projectId ? projectId : '')
  const projectName = $derived(projects.find((p) => p.id === projectId)?.name ?? '')

  let seq = 0
  let timer = 0
  $effect(() => {
    // Re-run on query, scope and stamp changes (debounced while typing).
    const q = query
    const pid = scopeId
    void stamp
    clearTimeout(timer)
    timer = window.setTimeout(() => void search(q, pid), q ? 200 : 0)
    return () => clearTimeout(timer)
  })

  async function search(q: string, pid: string) {
    const mine = ++seq
    loading = true
    try {
      const r = await api.searchHistory(q, pid, 100)
      if (mine !== seq) return
      hits = r
      error = ''
      if (picked && !r.some((h) => h.entry.id === picked)) {
        picked = null
        detail = null
      }
    } catch (e) {
      if (mine === seq) error = errorMessage(e)
    } finally {
      if (mine === seq) loading = false
    }
  }

  async function open(id: string) {
    picked = id
    confirmDelete = false
    detailError = ''
    try {
      const d = await api.historyDetail(id)
      if (picked !== id) return
      detail = d
      await tick()
      scrollToMatch()
    } catch (e) {
      if (picked === id) ((detail = null), (detailError = errorMessage(e)))
    }
  }

  /** The text pane starts at the first line that matches the search. */
  function scrollToMatch() {
    if (!textBox || !detail) return
    const lines = matchingLines(detail.text, query)
    const el = lines.length ? textBox.querySelector<HTMLElement>(`[data-line="${lines[0]}"]`) : null
    if (el) textBox.scrollTop = el.offsetTop - textBox.clientHeight / 3
    else textBox.scrollTop = textBox.scrollHeight
  }

  async function resume() {
    if (!detail) return
    busy = true
    try {
      await onResume(detail.entry.id)
    } catch (e) {
      detailError = errorMessage(e)
    } finally {
      busy = false
    }
  }

  async function remove() {
    if (!detail) return
    busy = true
    try {
      await api.deleteHistory(detail.entry.id)
      hits = hits.filter((h) => h.entry.id !== detail?.entry.id)
      picked = null
      detail = null
    } catch (e) {
      detailError = errorMessage(e)
    } finally {
      busy = false
      confirmDelete = false
    }
  }

  function when(iso: string): string {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''
    const today = d.toDateString() === new Date().toDateString()
    return d.toLocaleString(locale(), today ? { hour: '2-digit', minute: '2-digit' } : { dateStyle: 'medium', timeStyle: 'short' })
  }

  function title(h: { name?: string; agent: string }): string {
    return h.name || h.agent
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      if (confirmDelete) confirmDelete = false
      else onClose()
    }
  }

  $effect(() => {
    input?.focus()
  })

  const textLines = $derived(detail ? detail.text.split('\n') : [])
  const hitLines = $derived(new Set(detail ? matchingLines(detail.text, query) : []))
</script>

<svelte:window onkeydown={onKeydown} />

<div class="history">
  <header class="titlebar drag">
    <span class="title">{t('history.title')}</span>
    <input
      class="search"
      type="search"
      bind:this={input}
      bind:value={query}
      placeholder={t('history.search')}
      spellcheck="false"
      autocomplete="off"
    />
    {#if projectId}
      <div class="scope" role="group">
        <button class:active={scope === 'project'} onclick={() => (scope = 'project')}>{projectName}</button>
        <button class:active={scope === 'all'} onclick={() => (scope = 'all')}>{t('history.allProjects')}</button>
      </div>
    {/if}
    <button class="close" title={t('history.close')} onclick={onClose}>×</button>
  </header>

  <div class="body">
    <aside class="list" aria-busy={loading}>
      {#if error}
        <p class="note error">{error}</p>
      {:else if !hits.length && !loading}
        <p class="note">{query ? t('history.noMatches') : t('history.empty')}</p>
      {/if}
      {#each hits as h (h.entry.id)}
        <button class="hit" class:active={picked === h.entry.id} onclick={() => open(h.entry.id)}>
          <div class="hit-head">
            <span class="hit-title">{title(h.entry)}</span>
            <span class="hit-when">{when(h.entry.endedAt)}</span>
          </div>
          <div class="hit-meta">
            {#if scope === 'all'}{h.entry.projectName}{' · '}{/if}{h.entry.agent}{#if h.entry.branches?.length}{' · '}<span
                class="mono">{h.entry.branches[0]}</span
              >{/if}
          </div>
          {#each h.snippets.filter((s) => s.source !== 'info') as sn, i (i)}
            <div class="snippet" class:prompt={sn.source === 'prompt'}>
              {#each highlight(sn.line, sn.matches) as part, j (j)}{#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}
            </div>
          {/each}
          {#if h.count > h.snippets.length}
            <div class="more">{t('history.moreMatches', { n: h.count - h.snippets.length })}</div>
          {/if}
        </button>
      {/each}
    </aside>

    <section class="detail">
      {#if detail}
        {@const e = detail.entry}
        <div class="detail-head">
          <div>
            <h2>{title(e)}</h2>
            <p class="meta">
              {e.projectName} · {e.agent}{#if e.model} · {e.model}{/if} · {when(e.createdAt)} ({duration(e.createdAt, e.endedAt)})
            </p>
            <p class="meta mono" title={e.cwd}>{e.cwd}</p>
            {#if e.branches?.length}
              <p class="meta">{t('history.branches')} <span class="mono">{e.branches.join(', ')}</span></p>
            {/if}
          </div>
          <div class="actions">
            {#if detail.canResume}
              <button class="btn primary" disabled={busy} title={t('history.resumeTitle')} onclick={resume}>{t('history.resume')}</button>
            {/if}
            {#if confirmDelete}
              <span class="confirm">{t('history.deleteAsk')}</span>
              <button class="btn danger" disabled={busy} onclick={remove}>{t('history.delete')}</button>
              <button class="btn" disabled={busy} onclick={() => (confirmDelete = false)}>{t('history.cancel')}</button>
            {:else}
              <button class="btn" disabled={busy} onclick={() => (confirmDelete = true)}>{t('history.delete')}</button>
            {/if}
          </div>
        </div>
        {#if detailError}<p class="note error">{detailError}</p>{/if}
        {#if e.prompts?.length}
          <details class="prompts" open={e.prompts.length <= 12}>
            <summary>{t('history.prompts', { n: e.prompts.length })}</summary>
            <ol>
              {#each e.prompts as p, i (i)}<li>{p}</li>{/each}
            </ol>
          </details>
        {/if}
        <pre class="text" bind:this={textBox}>{#each textLines as l, i (i)}<span data-line={i} class:line-hit={hitLines.has(i)}>{l}
</span>{/each}</pre>
      {:else if detailError}
        <p class="note error">{detailError}</p>
      {:else}
        <p class="note">{t('history.pick')}</p>
      {/if}
    </section>
  </div>
</div>

<style>
  .history {
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
  .search {
    flex: 1;
    max-width: 420px;
    padding: 4px 8px;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    background: var(--input-bg, transparent);
    color: var(--fg);
  }
  .scope {
    display: flex;
    gap: 2px;
  }
  .scope button {
    padding: 3px 10px;
    border-radius: 5px;
    color: var(--fg-dim);
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .scope button:hover {
    background: var(--hover);
  }
  .scope button.active {
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
  .body {
    flex: 1;
    min-height: 0;
    display: flex;
  }
  .list {
    width: 380px;
    flex: none;
    overflow-y: auto;
    border-right: 1px solid var(--border);
    padding: 6px;
  }
  .hit {
    display: block;
    width: 100%;
    text-align: left;
    padding: 7px 9px;
    border-radius: 7px;
    margin-bottom: 2px;
  }
  .hit:hover {
    background: var(--hover);
  }
  .hit.active {
    background: var(--accent-bg);
  }
  .hit-head {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .hit-title {
    flex: 1;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hit-when,
  .hit-meta,
  .more {
    font-size: 11px;
    color: var(--muted);
    white-space: nowrap;
  }
  .hit-meta {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .snippet {
    margin-top: 3px;
    font: 11px var(--mono);
    color: var(--fg-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .snippet.prompt {
    font-family: inherit;
    font-size: 12px;
    color: var(--fg);
  }
  .snippet.prompt::before {
    content: '› ';
    color: var(--muted);
  }
  mark {
    background: var(--amber);
    color: var(--bg);
    border-radius: 2px;
    padding: 0 1px;
  }
  .detail {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    padding: 12px 16px;
    gap: 8px;
  }
  .detail-head {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    justify-content: space-between;
  }
  h2 {
    margin: 0 0 2px;
    font-size: 15px;
  }
  .meta {
    margin: 0;
    font-size: 11.5px;
    color: var(--muted);
  }
  .mono {
    font-family: var(--mono);
  }
  .actions {
    display: flex;
    gap: 8px;
    align-items: center;
    flex: none;
  }
  .confirm {
    font-size: 12px;
    color: var(--fg-dim);
  }
  .btn {
    padding: 4px 12px;
    border-radius: 6px;
    border: 1px solid var(--border-strong);
  }
  .btn:hover:not(:disabled) {
    background: var(--hover);
  }
  .btn.primary {
    background: var(--accent-bg);
  }
  .btn.danger {
    color: var(--red);
  }
  .prompts {
    font-size: 12.5px;
    max-height: 30%;
    overflow-y: auto;
    flex: none;
  }
  .prompts summary {
    cursor: default;
    color: var(--fg-dim);
  }
  .prompts ol {
    margin: 6px 0 0;
    padding-left: 24px;
  }
  .prompts li {
    white-space: pre-wrap;
    margin-bottom: 4px;
  }
  .text {
    flex: 1;
    min-height: 0;
    margin: 0;
    overflow: auto;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    font: 11.5px/1.4 var(--mono);
    white-space: pre;
    color: var(--fg-dim);
  }
  .text .line-hit {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .note {
    color: var(--muted);
    font-size: 12px;
    padding: 8px;
  }
  .note.error {
    color: var(--red);
  }
</style>
