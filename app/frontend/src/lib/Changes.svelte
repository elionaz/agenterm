<script lang="ts">
  // "Cambios" view: a session's (or an orphaned worktree's) diff — file list,
  // commits ahead of the base, the selected file rendered with diff2html —
  // and the worktree actions (editor, Finder, copy branch, merge, remove).
  import { onMount } from 'svelte'
  import { html as renderDiff } from 'diff2html'
  import type { ColorSchemeType } from 'diff2html/lib/types'
  import 'diff2html/bundles/css/diff2html.min.css'
  import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
  import { api, errorMessage, type DiffFile, type DiffResult, type LossInfo } from './api'

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
  // Merge confirmation.
  let merging = $state(false)

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
      removing = merging = false
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
    flash(`Copiado: ${diff.branch}`)
  }

  async function doMerge() {
    if (!wtId) return
    busy = true
    try {
      const r = await api.mergeWorktree(wtId)
      merging = false
      let msg = `${r.branch} mergeada en ${r.base} (${r.commits} ${r.commits === 1 ? 'commit' : 'commits'})`
      if (r.uncommitted) msg += ` · ojo: ${r.uncommitted} cambios sin commitear no se incluyeron`
      flash(msg)
      void load()
    } catch (e) {
      onError(errorMessage(e))
    } finally {
      busy = false
    }
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

  function askRemove() {
    merging = false
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
      <span class="branch" title={diff.dir}>⎇ {diff.branch}{#if isWorktree && diff.base}<span class="base"> ← {diff.base}</span>{/if}</span>
    {/if}
    {#if diff}
      <span class="stats">
        {diff.files.length}
        {diff.files.length === 1 ? 'archivo' : 'archivos'}
        <span class="add">+{diff.additions}</span>
        <span class="del">−{diff.deletions}</span>
        {#if diff.git.ahead}<span class="ahead">· {diff.git.ahead} {diff.git.ahead === 1 ? 'commit' : 'commits'} por delante</span>{/if}
      </span>
    {/if}
    <button class="icon" class:spin={loading} title="Actualizar" aria-label="Actualizar" onclick={load}>⟳</button>
    <span class="seg">
      <button class:on={style === 'side-by-side'} onclick={() => setStyle('side-by-side')}>Lado a lado</button>
      <button class:on={style === 'line-by-line'} onclick={() => setStyle('line-by-line')}>Unificado</button>
    </span>
    <span class="spacer"></span>
    <span class="actions">
      <button class="btn" onclick={() => open('editor')}>Abrir en editor</button>
      <button class="btn" onclick={() => open('finder')}>Abrir en Finder</button>
      {#if diff?.branch}<button class="btn" onclick={copyBranch}>Copiar rama</button>{/if}
      {#if isWorktree && wtId}
        <button class="btn" disabled={busy} onclick={() => ((merging = !merging), (removing = false))}>Merge a {diff?.base || 'base'}</button>
        <button
          class="btn danger"
          disabled={busy || live}
          title={live ? 'Termina la sesión antes de borrar su worktree' : ''}
          onclick={askRemove}>Eliminar worktree</button
        >
      {/if}
      {#if onClose}<button class="icon close" title="Cerrar (Esc)" aria-label="Cerrar" onclick={onClose}>×</button>{/if}
    </span>
  </header>

  {#if notice}<div class="notice ok">{notice}</div>{/if}

  {#if merging && diff}
    <div class="confirm">
      <p>
        Merge <code>--no-ff</code> de <b>{diff.branch}</b> en <b>{diff.base}</b>, en la carpeta principal del proyecto. Solo
        se hace si está en <b>{diff.base}</b> y sin cambios sin commitear (no hay stash automático); si hay conflictos, se aborta.
        {#if diff.git.uncommitted}<br /><span class="warn">Hay {diff.git.uncommitted} cambios sin commitear en el worktree: no se incluyen.</span>{/if}
      </p>
      <div class="row">
        <button class="btn primary" disabled={busy || !diff.git.ahead} onclick={doMerge}>
          {diff.git.ahead ? `Hacer merge (${diff.git.ahead} ${diff.git.ahead === 1 ? 'commit' : 'commits'})` : 'Nada que mergear'}
        </button>
        <button class="btn" onclick={() => (merging = false)}>Cancelar</button>
      </div>
    </div>
  {/if}

  {#if removing}
    <div class="confirm danger-zone">
      <p>Se borra la carpeta del worktree{diff?.dir ? ` (${diff.dir})` : ''}. Tu carpeta del proyecto no se toca.</p>
      <label class="check"><input type="checkbox" bind:checked={deleteBranch} onchange={checkLoss} /> Borrar también la rama {diff?.branch ?? ''}</label>
      {#if lossLoading}
        <p class="muted">Comprobando qué se perdería…</p>
      {:else if loss}
        <div class="loss">
          {#if loss.uncommitted?.length}
            <b>Se perderían {loss.uncommitted.length} {loss.uncommitted.length === 1 ? 'archivo' : 'archivos'} sin commitear:</b>
            <ul>
              {#each loss.uncommitted.slice(0, 12) as f (f)}<li><code>{f}</code></li>{/each}
              {#if loss.uncommitted.length > 12}<li>… y {loss.uncommitted.length - 12} más</li>{/if}
            </ul>
          {/if}
          {#if loss.unmerged?.length}
            <b>Y {loss.unmerged.length} {loss.unmerged.length === 1 ? 'commit' : 'commits'} sin mergear en {loss.base}:</b>
            <ul>
              {#each loss.unmerged.slice(0, 8) as c (c.hash)}<li><code>{c.short}</code> {c.subject}</li>{/each}
            </ul>
          {/if}
          <label class="check"><input type="checkbox" bind:checked={force} /> Borrar igualmente (no se puede deshacer)</label>
        </div>
      {:else}
        <p class="muted">No se pierde nada{deleteBranch ? '' : ': la rama y sus commits se conservan'}.</p>
      {/if}
      <div class="row">
        <button class="btn danger" disabled={busy || lossLoading || (!!loss && !force)} onclick={doRemove}>Eliminar</button>
        <button class="btn" onclick={() => (removing = false)}>Cancelar</button>
      </div>
    </div>
  {/if}

  {#if loadError && !diff}
    <div class="empty">
      <p class="err">{loadError}</p>
      <button class="btn" onclick={load}>Reintentar</button>
    </div>
  {:else if !diff}
    <div class="empty"><p class="muted">Cargando cambios…</p></div>
  {:else}
    {#if diff.truncated}
      <div class="notice">El diff es muy grande (más de 2 MB): algunos archivos se muestran sin contenido. Ábrelo en el editor para verlo entero.</div>
    {/if}
    <div class="body">
      <aside class="files">
        {#if diff.commits.length}
          <div class="section">Commits ({diff.commits.length})</div>
          <ul class="commits">
            {#each diff.commits as c (c.hash)}
              <li title={`${c.hash}\n${c.author} · ${new Date(c.date).toLocaleString()}`}><code>{c.short}</code> {c.subject}</li>
            {/each}
          </ul>
        {/if}
        <div class="section">Archivos ({diff.files.length})</div>
        {#each diff.files as f (f.path)}
          {@const n = fileName(f.path)}
          <button class="file" class:sel={selected?.path === f.path} title={f.oldPath ? `${f.oldPath} → ${f.path}` : f.path} onclick={() => (selectedPath = f.path)}>
            <span class={statusClass(f)}>{f.status}</span>
            <span class="fname"><span class="fdir">{n.dir}</span>{n.base}</span>
            {#if f.binary}<span class="tag">bin</span>
            {:else}<span class="add">+{f.additions}</span><span class="del">−{f.deletions}</span>{/if}
          </button>
        {:else}
          <p class="muted pad">Sin cambios{isWorktree ? ` respecto a ${diff.base}` : ''}.</p>
        {/each}
      </aside>
      <section class="diff">
        {#if selected}
          {#if selected.binary}
            <div class="empty"><p class="muted">{selected.path}: archivo binario (sin vista de contenido).</p></div>
          {:else if selected.truncated || !selected.patch}
            <div class="empty">
              <p class="muted">{selected.path}: sin contenido {selected.truncated ? '(límite de tamaño del diff)' : '(solo cambio de modo o renombrado)'}.</p>
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
  code {
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
