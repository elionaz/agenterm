<script lang="ts">
  // "Continue with…": hand a session's work to another agent (or to the
  // same one in a worktree of its own). The daemon drafts a summary (what
  // the user asked, where it worked, its changes and commits, the end of
  // its terminal); it is shown here to edit, then the new session starts
  // with it and the old one closes unless "keep" is ticked.
  import { onMount } from 'svelte'
  import { api, errorMessage, launchable, sessionTitle, type Profile, type Session, type SpawnMode } from './api'
  import { t } from './i18n.svelte'
  import { SHELL_PROFILE, folderSharers, inProjectFolder } from './shared'

  interface Props {
    session: Session
    /** Every session (the live ones of its project can take the work over). */
    sessions: Session[]
    profiles: Profile[]
    /** Preselected agent and place (Changes' "move to a worktree"). */
    preset?: { profileId?: string; where?: SpawnMode }
    cols: number
    rows: number
    onDone: (s: Session) => void
    onClose: () => void
  }
  let { session, sessions, profiles, preset, cols, rows, onDone, onClose }: Props = $props()

  const choices = $derived(launchable(profiles).filter((p) => p.id !== SHELL_PROFILE))
  const inWorktree = $derived(session.mode === 'worktree' || !!session.worktreeId)
  /** Live agents of the project that can take the work over. */
  const targets = $derived(
    sessions.filter(
      (s) => s.projectId === session.projectId && s.id !== session.id && s.profileId !== SHELL_PROFILE && s.state !== 'exited' && s.state !== 'suspended',
    ),
  )
  // svelte-ignore state_referenced_locally
  let into = $state<'new' | 'existing'>('new')
  let targetId = $state('')
  function targetLabel(s: Session): string {
    const place = s.mode === 'worktree' && s.branch ? s.branch : (s.git?.head ?? '')
    return place ? `${sessionTitle(s)} · ${place}` : sessionTitle(s)
  }
  // svelte-ignore state_referenced_locally
  let profileId = $state(preset?.profileId ?? '')
  // svelte-ignore state_referenced_locally
  let where = $state<SpawnMode>(preset?.where ?? 'same')
  let keep = $state(false)
  let note = $state('')
  let shared = $state(false)
  let loading = $state(true)
  let busy = $state(false)
  let error = $state<string | null>(null)
  const ready = $derived(!busy && !loading && !!note.trim() && (into === 'existing' ? !!targetId : !!profileId))

  onMount(async () => {
    if (!profileId) profileId = choices.find((p) => p.id !== session.profileId)?.id ?? choices[0]?.id ?? ''
    // In a shared folder, the agent already working there is the natural
    // one to take over (unless the caller asked for a new worktree).
    const sharer = inProjectFolder(session) ? folderSharers(sessions, session.projectId, session.id)[0] : undefined
    targetId = sharer?.id ?? targets[0]?.id ?? ''
    if (sharer && !preset?.where) into = 'existing'
    try {
      const n = await api.handoffNote(session.id)
      note = n.text
      shared = !!n.shared
      if (shared && !inWorktree && !preset?.where) where = 'worktree'
    } catch (e) {
      error = errorMessage(e)
    } finally {
      loading = false
    }
  })

  async function go() {
    busy = true
    error = null
    try {
      const to = into === 'existing' ? { targetId } : { profileId, where }
      onDone(await api.handoff(session.id, to, note, keep, cols, rows))
    } catch (e) {
      error = errorMessage(e)
    } finally {
      busy = false
    }
  }

  function onKey(e: KeyboardEvent) {
    e.stopPropagation()
    if (e.key === 'Escape') onClose()
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && ready) void go()
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}>
  <div class="card" role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="handoff-title" onclick={(e) => e.stopPropagation()} onkeydown={onKey}>
    <h2 id="handoff-title">{t('handoff.title')}</h2>
    <p>{t('handoff.info')}</p>

    <div class="grid">
      <span>{t('handoff.into')}</span>
      <span class="radios">
        <label><input type="radio" bind:group={into} value="new" /> {t('handoff.intoNew')}</label>
        <label><input type="radio" bind:group={into} value="existing" disabled={!targets.length} /> {t('handoff.intoExisting')}</label>
      </span>
      {#if into === 'existing'}
        <label for="handoff-target">{t('handoff.target')}</label>
        <select id="handoff-target" bind:value={targetId}>
          {#each targets as s (s.id)}<option value={s.id}>{targetLabel(s)}</option>{/each}
        </select>
      {:else}
        <label for="handoff-agent">{t('handoff.agent')}</label>
        <select id="handoff-agent" bind:value={profileId}>
          {#each choices as p (p.id)}<option value={p.id}>{p.name || p.id}{p.id === session.profileId ? ` (${t('handoff.same')})` : ''}</option>{/each}
        </select>

        <span>{t('handoff.where')}</span>
        <span class="radios">
          <label><input type="radio" bind:group={where} value="same" /> {inWorktree ? t('handoff.sameWorktree') : t('handoff.sameFolder')}</label>
          <label><input type="radio" bind:group={where} value="worktree" /> {t('handoff.newWorktree')}</label>
        </span>
      {/if}
    </div>
    {#if into === 'existing'}
      <p class="muted">{t('handoff.existingNote')}</p>
    {:else if where === 'worktree'}
      <p class="muted">{t('handoff.worktreeNote')}</p>
    {:else if shared}
      <p class="warn">{t('handoff.sharedNote')}</p>
    {/if}

    <label class="notelabel" for="handoff-note">{t('handoff.note')}</label>
    {#if loading}
      <p class="muted">{t('handoff.loading')}</p>
    {:else}
      <textarea id="handoff-note" rows="14" bind:value={note} spellcheck="false"></textarea>
    {/if}
    <label class="keep"><input type="checkbox" bind:checked={keep} /> {t('handoff.keep')}</label>
    {#if error}<div class="error">{error}</div>{/if}
    <div class="row">
      <button class="btn primary" disabled={!ready} onclick={go}>{t('handoff.go')}</button>
      <span class="grow"></span>
      <button class="btn" onclick={onClose}>{t('handoff.cancel')}</button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    background: color-mix(in srgb, var(--bg) 55%, transparent);
  }
  .card {
    width: min(680px, calc(100vw - 32px));
    max-height: calc(100vh - 48px);
    overflow: auto;
    padding: 16px 18px;
    border-radius: 10px;
    background: var(--menu-bg);
    border: 1px solid var(--border-strong);
    box-shadow: 0 12px 32px var(--shadow);
    font-size: 13px;
    color: var(--fg);
  }
  h2 {
    margin: 0 0 6px;
    font-size: 14px;
  }
  p {
    margin: 0 0 10px;
    color: var(--fg-dim);
  }
  .muted {
    color: var(--muted);
    font-size: 12px;
  }
  .warn {
    color: var(--amber);
    font-size: 12px;
  }
  .grid {
    display: grid;
    grid-template-columns: 90px 1fr;
    gap: 8px 10px;
    align-items: center;
    margin-bottom: 8px;
  }
  .radios {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 16px;
  }
  .radios label,
  .keep {
    display: flex;
    gap: 6px;
    align-items: center;
  }
  input[type='radio'],
  input[type='checkbox'] {
    accent-color: var(--blue);
  }
  select,
  textarea {
    font: inherit;
    font-size: 12.5px;
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    padding: 4px 6px;
  }
  .notelabel {
    display: block;
    margin: 6px 0 4px;
    color: var(--fg-dim);
    font-size: 12px;
  }
  textarea {
    display: block;
    width: 100%;
    box-sizing: border-box;
    font-family: var(--mono);
    font-size: 11.5px;
    line-height: 1.4;
    resize: vertical;
  }
  .keep {
    margin: 10px 0;
    font-size: 12.5px;
    color: var(--fg-dim);
  }
  .error {
    margin: 0 0 10px;
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--danger-bg);
    color: var(--danger-fg);
    border: 1px solid var(--danger-border);
    font-size: 12px;
    user-select: text;
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .grow {
    flex: 1;
  }
  .btn {
    padding: 4px 10px;
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
    opacity: 0.5;
  }
  .btn.primary {
    background: var(--accent-bg);
    color: var(--fg);
  }
</style>
