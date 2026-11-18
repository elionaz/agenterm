<script lang="ts">
  // "Changes" view: a session's (or an orphaned worktree's) diff — file list,
  // commits ahead of the base, the selected file rendered with diff2html —
  // and the worktree actions (editor, Finder, copy branch, merge, pull
  // request, remove).
  import { onMount } from 'svelte'
  import { html as renderDiff } from 'diff2html'
  import type { ColorSchemeType } from 'diff2html/lib/types'
  import 'diff2html/bundles/css/diff2html.min.css'
  import { BrowserOpenURL, ClipboardSetText } from '../../wailsjs/runtime/runtime'
  import { api, errorMessage, type DiffFile, type DiffResult, type LossInfo, type PRResult } from './api'
  import { locale, t, th } from './i18n.svelte'
  import PullRequest from './PullRequest.svelte'

  interface Props {
    /** What to diff: a session, or a worktree without one. */
    sessionId?: string
    worktreeId?: string
    /** A live session uses the worktree (it cannot be removed). */
    live?: boolean
    /** Changes whenever the daemon reports new git info: reload. */
    gitStamp?: string
    /** Open the remove confirmation right away. */
    startRemove?: boolean
    onError: (msg: string) => void
    /** The worktree was removed. */
    onRemoved?: () => void
    /** Shown as a close button (orphan overlay). */
    onClose?: () => void
  }

  let { sessionId, worktreeId, live = false, gitStamp, startRemove = false, onError, onRemoved, onClose }: Props = $props()

  const STYLE_KEY = 'agenterm.diffStyle'
  let diff = $state<DiffResult | null>(null)
  let loading = $state(false)
  let loadError = $state<string | null>(null)
  let selectedPath = $state<string | null>(null)
  let style = $state<'side-by-side' | 'line-by-line'>(readStyle())
  let notice = $state<string | null>(null)

  // Remove confirmation.
  let removing = $state(false)
  let deleteBranch = $state(false)
  let force = $state(false)
  let loss = $state<LossInfo | null>(null)
  let lossLoading = $state(false)
  let busy = $state(false)
  // Merge confirmation, and the offer to remove the worktree after a merge.
  let merging = $state(false)
  let removeOffer = $state(false)
  /** The merge result, shown in the remove offer (not as a timed notice, whose
   * disappearing would move the offer's buttons under the pointer). */
  let mergedMsg = $state('')
  // Pull request confirmation: remote and URL from a dry run.
  let prOpen = $state(false)
  let prInfo = $state<PRResult | null>(null)
  let prLoading = $state(false)
  let prCheckError = $state<string | null>(null)
  let prSeq = 0 // drops stale dry-run answers
  /** gh / glab work for this branch: the PR block handles pull requests. */
  let forgeAvailable = $state(false)
  /** ⟳ also reloads the PR block (gitStamp changes reload both). */
  let refreshTick = $state(0)

  const wtId = $derived(diff?.worktreeId || worktreeId || '')
  const isWorktree = $derived(diff?.mode === 'worktree' || !!worktreeId)
  const selected = $derived(diff?.files.find((f) => f.path === selectedPath) ?? diff?.files[0] ?? null)
  const rendered = $derived.by(() => {
    if (!selected?.patch) return ''
    try {
      return renderDiff(selected.patch, {
        outputFormat: style,
        drawFileList: false,
        matching: 'lines',
        colorScheme: 'dark' as ColorSchemeType,
      })
    } catch (e) {
      return `<pre class="raw">${escapeHtml(selected.patch)}</pre>`
    }
  })

  function readStyle(): 'side-by-side' | 'line-by-line' {
    try {
      return localStorage.getItem(STYLE_KEY) === 'line-by-line' ? 'line-by-line' : 'side-by-side'
    } catch {
      return 'side-by-side'
    }
  }

  function setStyle(s: 'side-by-side' | 'line-by-line') {
    style = s
    try {
      localStorage.setItem(STYLE_KEY, s)
    } catch {
      /* ignore */
    }
  }

  function escapeHtml(s: string): string {
    return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
  }

  let seq = 0
  async function load() {
    const my = ++seq
    loading = true
    try {
      const d = sessionId ? await api.sessionDiff(sessionId) : await api.worktreeDiff(worktreeId!)
      if (my !== seq) return
      diff = d
      loadError = null
      if (!d.files.some((f) => f.path === selectedPath)) selectedPath = d.files[0]?.path ?? null
    } catch (e) {
      if (my === seq) loadError = errorMessage(e)
    } finally {
      if (my === seq) loading = false
    }
  }

  // Reload when the target changes or the daemon reports new git info.
  let lastKey = ''
  $effect(() => {
    const key = `${sessionId ?? ''}|${worktreeId ?? ''}|${gitStamp ?? ''}`
    if (key === lastKey) return
    const targetChanged = lastKey.split('|').slice(0, 2).join('|') !== key.split('|').slice(0, 2).join('|')
    lastKey = key
    if (targetChanged) {
      diff = null
      selectedPath = null
      removing = merging = prOpen = removeOffer = false
      prSeq++
      prLoading = false
    }
    void load()
  })

  function statusClass(f: DiffFile): string {
    return `st st-${f.status}`
  }

  function fileName(path: string): { dir: string; base: string } {
    const i = path.lastIndexOf('/')
    return i < 0 ? { dir: '', base: path } : { dir: path.slice(0, i + 1), base: path.slice(i + 1) }
  }

  function flash(msg: string) {
    notice = msg
    window.setTimeout(() => notice === msg && (notice = null), 4000)
  }

  async function open(target: 'editor' | 'finder') {
    try {
      await api.openFolder(target, wtId ? { worktreeId: wtId } : { sessionId })
    } catch (e) {
      onError(errorMessage(e))
    }
  }

  async function copyBranch() {
    if (!diff?.branch) return
    await ClipboardSetText(diff.branch)
    flash(t('changes.copied', { branch: diff.branch }))
  }

  function toggleMerge() {
    merging = !merging
    removing = prOpen = removeOffer = false
  }

  async function doMerge() {
    if (!wtId) return
    busy = true
    try {
      const r = await api.mergeWorktree(wtId)
      merging = false
      let msg = t('changes.merged', { branch: r.branch, base: r.base, n: r.commits })
      if (r.uncommitted) msg += t('changes.mergedUncommitted', { n: r.uncommitted })
      if (live) flash(msg)
      else ((mergedMsg = msg), (removeOffer = true))
      void load()
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      busy = false
    }
  }

  async function togglePR() {
    if (prOpen) {
      prOpen = false
      prSeq++
      prLoading = false
      return
    }
    merging = removing = removeOffer = false
    prOpen = true
    prInfo = null
    prCheckError = null
    if (!wtId || !diff?.git.ahead) return
    const my = ++prSeq
    const id = wtId
    prLoading = true
    try {
      const r = await api.pullRequest(id, true)
      if (my === prSeq && id === wtId) prInfo = r
    } catch (e) {
      if (my === prSeq && id === wtId) prCheckError = errorMessage(e)
    } finally {
      if (my === prSeq && id === wtId) prLoading = false
    }
  }

  async function doPR() {
    if (!wtId) return
    busy = true
    try {
      const r = await api.pullRequest(wtId)
      prOpen = false
      let msg = t(r.url ? 'changes.prOpened' : 'changes.prPushed', { branch: r.branch, remote: r.remote })
      if (r.uncommitted) msg += t('changes.prPushedUncommitted', { n: r.uncommitted })
      flash(msg)
      if (r.url) BrowserOpenURL(r.url)
      void load()
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      busy = false
    }
  }

  /** After a merge: the remove confirmation, with the branch preselected. */
  function offerRemove() {
    askRemove(true)
  }

  async function checkLoss() {
    if (!wtId) return
    lossLoading = true
    try {
      const r = await api.removeWorktree(wtId, { deleteBranch, dryRun: true })
      loss = r.loss ?? null
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      lossLoading = false
    }
  }

  function askRemove(withBranch = false) {
    deleteBranch = withBranch
    merging = prOpen = removeOffer = false
    removing = true
    force = false
    void checkLoss()
  }

  async function doRemove() {
    if (!wtId) return
    busy = true
    try {
      await api.removeWorktree(wtId, { deleteBranch, force: force || !!loss })
      removing = false
      onRemoved?.()
    } catch (e) {
      onError(errorMessage(e))
      void checkLoss()
    } finally {
      busy = false
    }
  }

  onMount(() => {
    if (startRemove) askRemove()
  })
</script>

<div class="changes">
  <header class="bar">
    {#if diff?.branch}
      <span class="branch" title={diff.dir}>⎇ {diff.branch}{#if isWorktree && diff.base}<span class="base">{` ← ${diff.base}`}</span>{/if}</span>
    {/if}
    {#if diff}
      <span class="stats">
        {t('changes.files', { n: diff.files.length })}
        <span class="add">+{diff.additions}</span>
        <span class="del">−{diff.deletions}</span>
        {#if diff.git.ahead}<span class="ahead">{t('changes.ahead', { n: diff.git.ahead })}</span>{/if}
      </span>
    {/if}
    <button class="icon" class:spin={loading} title={t('changes.refresh')} aria-label={t('changes.refresh')} onclick={() => {
        refreshTick++
        void load()
      }}>⟳</button>
    <span class="seg">
      <button class:on={style === 'side-by-side'} onclick={() => setStyle('side-by-side')}>{t('changes.sideBySide')}</button>
      <button class:on={style === 'line-by-line'} onclick={() => setStyle('line-by-line')}>{t('changes.unified')}</button>
    </span>
    <span class="spacer"></span>
    <span class="actions">
      <button class="btn" onclick={() => open('editor')}>{t('changes.openEditor')}</button>
      <button class="btn" onclick={() => open('finder')}>{t('changes.openFinder')}</button>
      {#if diff?.branch}<button class="btn" onclick={copyBranch}>{t('changes.copyBranch')}</button>{/if}
      {#if isWorktree && wtId}
        <button class="btn" disabled={busy} onclick={toggleMerge}>{t('changes.mergeInto', { base: diff?.base || 'base' })}</button>
        {#if !forgeAvailable}<button class="btn" disabled={busy} onclick={togglePR}>{t('changes.createPR')}</button>{/if}
        <button
          class="btn danger"
          disabled={busy || live}
          title={live ? t('changes.stopFirst') : ''}
          onclick={() => askRemove()}>{t('changes.removeWorktree')}</button
        >
      {/if}
      {#if onClose}<button class="icon close" title={t('changes.close')} aria-label={t('changes.closeLabel')} onclick={onClose}>×</button>{/if}
    </span>
  </header>

  {#if diff?.branch}
    <PullRequest
      {sessionId}
      {worktreeId}
      reloadKey={`${gitStamp ?? ''}|${refreshTick}`}
      {onError}
      onSelectFile={(p) => {
        if (diff?.files.some((f) => f.path === p)) selectedPath = p
      }}
      onMerged={isWorktree
        ? (msg) => {
            mergedMsg = msg
            merging = prOpen = removing = false
            removeOffer = true
          }
        : undefined}
      bind:available={forgeAvailable}
    />
  {/if}

  {#if notice}<div class="notice ok">{notice}</div>{/if}

  {#if merging && diff}
    <div class="confirm">
      <p>
        {@html th('changes.mergeInfo', { branch: diff.branch ?? '', base: diff.base ?? '' })}
        {#if diff.git.uncommitted}<br /><span class="warn">{t('changes.mergeUncommitted', { n: diff.git.uncommitted })}</span>{/if}
      </p>
      <div class="row">
        <button class="btn primary" disabled={busy || !diff.git.ahead} onclick={doMerge}>
          {diff.git.ahead ? t('changes.doMerge', { n: diff.git.ahead }) : t('changes.nothingToMerge')}
        </button>
        <button class="btn" onclick={() => (merging = false)}>{t('changes.cancel')}</button>
      </div>
    </div>
  {/if}

  {#if removeOffer && !merging && !removing}
    <div class="confirm">
      {#if mergedMsg}<p class="ok">{mergedMsg}</p>{/if}
      <p>{t('changes.removeOffer')}</p>
      <div class="row">
        <button class="btn danger" disabled={busy || live} onclick={offerRemove}>{t('changes.removeOfferYes')}</button>
        <button class="btn" onclick={() => (removeOffer = false)}>{t('changes.removeOfferNo')}</button>
      </div>
    </div>
  {/if}

  {#if prOpen && diff}
    <div class="confirm">
      {#if prLoading}
        <p class="muted">{t('changes.prChecking')}</p>
      {:else if prCheckError}
        <p class="err">{prCheckError}</p>
      {:else if prInfo}
        <p>
          {@html th('changes.prInfo', { branch: prInfo.branch, remote: prInfo.remote, base: prInfo.base })}
          {#if !prInfo.url}<br /><span class="muted">{t('changes.prNoUrl', { remote: prInfo.remote })}</span>{/if}
          {#if prInfo.uncommitted}<br /><span class="warn">{t('changes.prUncommitted', { n: prInfo.uncommitted })}</span>{/if}
        </p>
      {/if}
      <div class="row">
        <button class="btn primary" disabled={busy || prLoading || !prInfo || !diff.git.ahead} onclick={doPR}>
          {diff.git.ahead ? t('changes.doPR', { n: diff.git.ahead }) : t('changes.nothingToPR')}
        </button>
        <button class="btn" onclick={() => (prOpen = false)}>{t('changes.cancel')}</button>
      </div>
    </div>
  {/if}

  {#if removing}
    <div class="confirm danger-zone">
      <p>{diff?.dir ? t('changes.removeInfoDir', { dir: diff.dir }) : t('changes.removeInfo')}</p>
      <label class="check"
        ><input type="checkbox" bind:checked={deleteBranch} onchange={checkLoss} /> {t('changes.deleteBranch', { branch: diff?.branch ?? '' })}</label
      >
      {#if lossLoading}
        <p class="muted">{t('changes.checkingLoss')}</p>
      {:else if loss}
        <div class="loss">
          {#if loss.uncommitted?.length}
            <b>{t('changes.lossFiles', { n: loss.uncommitted.length })}</b>
            <ul>
              {#each loss.uncommitted.slice(0, 12) as f (f)}<li><code>{f}</code></li>{/each}
              {#if loss.uncommitted.length > 12}<li>{t('changes.andMore', { n: loss.uncommitted.length - 12 })}</li>{/if}
            </ul>
          {/if}
          {#if loss.unmerged?.length}
            <b>{t('changes.lossCommits', { n: loss.unmerged.length, base: loss.base })}</b>
            <ul>
              {#each loss.unmerged.slice(0, 8) as c (c.hash)}<li><code>{c.short}</code> {c.subject}</li>{/each}
            </ul>
          {/if}
          <label class="check"><input type="checkbox" bind:checked={force} /> {t('changes.removeAnyway')}</label>
        </div>
      {:else}
        <p class="muted">{deleteBranch ? t('changes.nothingLost') : t('changes.nothingLostKeep')}</p>
      {/if}
      <div class="row">
        <button class="btn danger" disabled={busy || lossLoading || (!!loss && !force)} onclick={doRemove}>{t('changes.remove')}</button>
        <button class="btn" onclick={() => (removing = false)}>{t('changes.cancel')}</button>
      </div>
    </div>
  {/if}

  {#if loadError && !diff}
    <div class="empty">
      <p class="err">{loadError}</p>
      <button class="btn" onclick={load}>{t('changes.retry')}</button>
    </div>
  {:else if !diff}
    <div class="empty"><p class="muted">{t('changes.loading')}</p></div>
  {:else}
    {#if diff.truncated}
      <div class="notice">{t('changes.truncated')}</div>
    {/if}
    <div class="body">
      <aside class="files">
        {#if diff.commits.length}
          <div class="section">{t('changes.commits', { n: diff.commits.length })}</div>
          <ul class="commits">
            {#each diff.commits as c (c.hash)}
              <li title={`${c.hash}\n${c.author} · ${new Date(c.date).toLocaleString(locale())}`}><code>{c.short}</code> {c.subject}</li>
            {/each}
          </ul>
        {/if}
        <div class="section">{t('changes.fileList', { n: diff.files.length })}</div>
        {#each diff.files as f (f.path)}
          {@const n = fileName(f.path)}
          <button class="file" class:sel={selected?.path === f.path} title={f.oldPath ? `${f.oldPath} → ${f.path}` : f.path} onclick={() => (selectedPath = f.path)}>
            <span class={statusClass(f)}>{f.status}</span>
            <span class="fname"><span class="fdir">{n.dir}</span>{n.base}</span>
            {#if f.binary}<span class="tag">bin</span>
            {:else}<span class="add">+{f.additions}</span><span class="del">−{f.deletions}</span>{/if}
          </button>
        {:else}
          <p class="muted pad">{isWorktree ? t('changes.noChangesBase', { base: diff.base ?? '' }) : t('changes.noChanges')}</p>
        {/each}
      </aside>
      <section class="diff">
        {#if selected}
          {#if selected.binary}
            <div class="empty"><p class="muted">{t('changes.binary', { path: selected.path })}</p></div>
          {:else if selected.truncated || !selected.patch}
            <div class="empty">
              <p class="muted">
                {selected.truncated ? t('changes.noContentLimit', { path: selected.path }) : t('changes.noContentMode', { path: selected.path })}
              </p>
            </div>
          {:else}
            {@html rendered}
          {/if}
        {/if}
      </section>
    </div>
  {/if}
</div>

<style>
  .changes {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: flex;
    flex-direction: column;
    background: var(--bg);
    min-height: 0;
  }
  .bar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 12px;
    border-bottom: 1px solid var(--border);
    white-space: nowrap;
    overflow-x: auto;
  }
  .branch {
    font-family: var(--mono);
    font-size: 12px;
    color: var(--fg);
  }
  .base {
    color: var(--muted);
  }
  .stats {
    color: var(--fg-dim);
    font-size: 12px;
  }
  .add {
    color: var(--green);
    font-variant-numeric: tabular-nums;
  }
  .del {
    color: var(--red);
    font-variant-numeric: tabular-nums;
  }
  .ahead {
    color: var(--muted);
  }
  .spacer {
    flex: 1;
  }
  .actions {
    display: flex;
    gap: 6px;
    align-items: center;
  }
  .icon {
    width: 22px;
    height: 22px;
    border-radius: 5px;
    color: var(--muted);
    font-size: 14px;
  }
  .icon:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .icon.spin {
    animation: spin 0.8s linear infinite;
  }
  .close {
    font-size: 16px;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .seg {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: 6px;
    overflow: hidden;
  }
  .seg button {
    padding: 2px 8px;
    font-size: 11.5px;
    color: var(--fg-dim);
  }
  .seg button.on {
    background: var(--accent-bg);
    color: var(--fg);
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
  .btn.danger {
    color: var(--red);
    border-color: color-mix(in srgb, var(--red) 40%, transparent);
  }
  .notice {
    flex: none;
    padding: 6px 12px;
    font-size: 12px;
    color: var(--amber);
    background: color-mix(in srgb, var(--amber) 8%, transparent);
    border-bottom: 1px solid var(--border);
  }
  .notice.ok {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 8%, transparent);
  }
  .confirm .ok {
    color: var(--green);
  }
  .confirm {
    flex: none;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--sidebar-bg);
    font-size: 12.5px;
  }
  .confirm p {
    margin: 0 0 8px;
    color: var(--fg-dim);
  }
  .confirm .row {
    display: flex;
    gap: 8px;
    margin-top: 8px;
  }
  .danger-zone {
    box-shadow: inset 3px 0 0 var(--red);
  }
  .warn {
    color: var(--amber);
  }
  .check {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--fg-dim);
    margin: 4px 0;
  }
  .check input {
    accent-color: var(--red);
  }
  .loss {
    margin-top: 6px;
    color: var(--fg);
  }
  .loss ul {
    margin: 4px 0 8px;
    padding-left: 18px;
    max-height: 140px;
    overflow-y: auto;
    color: var(--fg-dim);
  }
  .muted {
    color: var(--muted);
  }
  .pad {
    padding: 8px 12px;
  }
  .err {
    color: var(--red);
  }
  .empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 20px;
    text-align: center;
  }
  .body {
    flex: 1;
    min-height: 0;
    display: flex;
  }
  .files {
    flex: none;
    width: 280px;
    overflow-y: auto;
    border-right: 1px solid var(--border);
    padding-bottom: 8px;
  }
  .section {
    padding: 8px 12px 4px;
    font-size: 10.5px;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .commits {
    list-style: none;
    margin: 0;
    padding: 0 12px 4px;
    font-size: 12px;
    color: var(--fg-dim);
  }
  .commits li {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding: 1px 0;
  }
  code,
  .confirm :global(code) {
    font-family: var(--mono);
    font-size: 11px;
    color: var(--blue);
  }
  .file {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 3px 12px;
    text-align: left;
    font-size: 12px;
    color: var(--fg-dim);
  }
  .file:hover {
    background: var(--hover);
  }
  .file.sel {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .fname {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .fdir {
    color: var(--muted);
  }
  .tag {
    font-size: 10px;
    color: var(--muted);
    border: 1px solid var(--border-strong);
    border-radius: 3px;
    padding: 0 3px;
  }
  .st {
    flex: none;
    width: 14px;
    font-family: var(--mono);
    font-weight: 700;
    font-size: 11px;
    text-align: center;
  }
  .st-A {
    color: var(--green);
  }
  .st-M,
  .st-T {
    color: var(--amber);
  }
  .st-D {
    color: var(--red);
  }
  .st-R,
  .st-C {
    color: var(--blue);
  }
  .diff {
    flex: 1;
    min-width: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
  }
  .diff :global(.raw) {
    margin: 0;
    padding: 10px;
    font-family: var(--mono);
    font-size: 12px;
  }
  /* diff2html, dark scheme tuned to the app palette. */
  .diff :global(.d2h-dark-color-scheme) {
    --d2h-dark-bg-color: var(--bg);
    --d2h-dark-color: var(--fg);
    --d2h-dark-file-header-bg-color: var(--sidebar-bg);
    --d2h-dark-file-header-border-color: var(--border);
    --d2h-dark-border-color: var(--border-strong);
    --d2h-dark-line-border-color: var(--border);
    --d2h-dark-dim-color: var(--muted);
    --d2h-dark-info-bg-color: color-mix(in srgb, var(--blue) 8%, transparent);
    --d2h-dark-ins-bg-color: color-mix(in srgb, var(--green) 12%, transparent);
    --d2h-dark-del-bg-color: color-mix(in srgb, var(--red) 12%, transparent);
    --d2h-dark-ins-highlight-bg-color: color-mix(in srgb, var(--green) 35%, transparent);
    --d2h-dark-del-highlight-bg-color: color-mix(in srgb, var(--red) 35%, transparent);
    --d2h-dark-ins-label-color: var(--green);
    --d2h-dark-del-label-color: var(--red);
    background: var(--bg);
  }
  .diff :global(.d2h-wrapper) {
    font-family: var(--mono);
  }
  .diff :global(.d2h-file-wrapper) {
    margin: 0;
    border: none;
    border-radius: 0;
  }
  .diff :global(.d2h-code-line),
  .diff :global(.d2h-code-side-line),
  .diff :global(.d2h-code-linenumber),
  .diff :global(.d2h-code-side-linenumber) {
    font-size: 12px;
  }
  .diff :global(.d2h-file-header) {
    position: sticky;
    top: 0;
    z-index: 1;
  }
</style>
