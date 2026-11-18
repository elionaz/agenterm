<script lang="ts">
  // Rename a project and pick its badge: a color (or automatic) and an
  // optional emoji in place of the initials.
  import type { Project } from './api'
  import { PROJECT_COLORS, type ProjectPatch } from './badge'
  import { t } from './i18n.svelte'
  import ProjectBadge from './ProjectBadge.svelte'

  interface Props {
    project: Project
    /** Saves the changed fields; rejects with a message to show. */
    onSave: (patch: ProjectPatch) => Promise<void>
    onClose: () => void
  }
  let { project, onSave, onClose }: Props = $props()

  // What the project was when the popover opened: only fields changed here
  // are sent, so a change made meanwhile (another window, the CLI) stays.
  // svelte-ignore state_referenced_locally
  const initial = { name: project.name, color: project.color ?? '', icon: project.icon ?? '' }
  let name = $state(initial.name)
  let color = $state(initial.color)
  let icon = $state(initial.icon)
  let error = $state('')
  let saving = $state(false)

  const preview = $derived({ id: project.id, name: name.trim() || project.name, color, icon })

  async function save() {
    const patch: ProjectPatch = {}
    if (name.trim() !== initial.name) patch.name = name
    if (color !== initial.color) patch.color = color
    if (icon.trim() !== initial.icon) patch.icon = icon.trim()
    if (!Object.keys(patch).length) return onClose()
    if (graphemeCount(icon.trim()) > 2) {
      error = t('err.project_icon_invalid')
      return
    }
    saving = true
    try {
      await onSave(patch)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      saving = false
    }
  }

  function onKey(e: KeyboardEvent) {
    e.stopPropagation() // keep Escape/Enter away from the window handlers
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
  function graphemeCount(s: string): number {
    return [...segmenter.segment(s)].length
  }

  function autofocus(node: HTMLInputElement) {
    node.focus()
    node.select()
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<form
  class="customize"
  aria-label={t('project.customize')}
  onkeydown={onKey}
  onsubmit={(e) => {
    e.preventDefault()
    void save()
  }}
>
  <div class="head">
    <ProjectBadge project={preview} size={28} />
    <input
      class="name"
      bind:value={name}
      maxlength="80"
      spellcheck="false"
      aria-label={t('project.name')}
      oninput={() => (error = '')}
      {@attach autofocus}
    />
  </div>

  <div class="label">{t('project.color')}</div>
  <div class="swatches" role="radiogroup" aria-label={t('project.color')}>
    <button
      type="button"
      class="swatch auto"
      role="radio"
      aria-checked={color === ''}
      title={t('project.auto')}
      onclick={() => (color = '')}>{t('project.auto')}</button
    >
    {#each PROJECT_COLORS as c (c)}
      <button
        type="button"
        class="swatch"
        role="radio"
        aria-checked={color === c}
        aria-label={t(`project.colors.${c}`)}
        title={t(`project.colors.${c}`)}
        style:--c="var(--{c})"
        onclick={() => (color = c)}
      ></button>
    {/each}
  </div>

  <label class="label" for="project-icon">{t('project.icon')}</label>
  <div class="icon-row">
    <input
      id="project-icon"
      class="icon-input"
      bind:value={icon}
      spellcheck="false"
      oninput={() => (error = '')}
    />
    {#if icon}
      <button type="button" class="clear" title={t('project.removeIcon')} onclick={() => (icon = '')}>✕</button>
    {/if}
    <span class="hint">{t('project.iconHint')}</span>
  </div>

  {#if error}<div class="error" role="alert">{error}</div>{/if}

  <div class="actions">
    <button type="button" onclick={onClose}>{t('project.cancel')}</button>
    <button type="submit" class="primary" disabled={saving || !name.trim()}>{t('project.save')}</button>
  </div>
</form>

<style>
  .customize {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 6px;
    font-weight: 400;
    font-size: 12px;
    color: var(--fg);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  input {
    min-width: 0;
    padding: 4px 6px;
    border: 1px solid var(--border-strong);
    border-radius: 5px;
    background: var(--bg);
    color: var(--fg);
    font: inherit;
  }
  .name {
    flex: 1;
    font-weight: 600;
  }
  .label {
    margin-top: 2px;
    color: var(--muted);
    font-size: 11px;
  }
  .swatches {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
  }
  .swatch {
    width: 15px;
    height: 15px;
    border-radius: 50%;
    background: var(--c);
    box-shadow: 0 0 0 1px var(--border-strong);
  }
  .swatch.auto {
    width: auto;
    height: 17px;
    padding: 0 6px;
    border-radius: 9px;
    background: var(--hover);
    color: var(--fg-dim);
    font-size: 11px;
  }
  .swatch[aria-checked='true'] {
    box-shadow:
      0 0 0 2px var(--menu-bg),
      0 0 0 4px var(--fg);
  }
  .icon-row {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .icon-input {
    width: 44px;
    text-align: center;
    font-size: 14px;
  }
  .clear {
    color: var(--muted);
  }
  .hint {
    color: var(--muted);
    font-size: 11px;
  }
  .error {
    padding: 4px 6px;
    border-radius: 5px;
    background: var(--danger-bg);
    color: var(--danger-fg);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 2px;
  }
  .actions button {
    padding: 3px 10px;
    border-radius: 5px;
    background: var(--hover);
    color: var(--fg);
  }
  .actions .primary {
    background: var(--blue);
    color: var(--badge-fg);
  }
  .actions .primary:disabled {
    opacity: 0.5;
  }
</style>
