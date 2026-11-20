<script lang="ts">
  // Asked before a new worktree runs a setup command the user has not
  // trusted yet (.panal.yaml's worktree.setup, error setup_untrusted):
  // a cloned repository could carry any command, so it is shown as is.
  import { t } from './i18n.svelte'

  interface Props {
    command: string
    /** The .panal.yaml that asks for it. */
    file: string
    onChoose: (choice: 'trust' | 'skip' | null) => void
  }
  let { command, file, onChoose }: Props = $props()
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onChoose(null)} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={() => onChoose(null)}>
  <div class="card" role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="trust-title" onclick={(e) => e.stopPropagation()}>
    <h2 id="trust-title">{t('setup.trustTitle')}</h2>
    <p>{t('setup.trustInfo', { file })}</p>
    <pre>{command}</pre>
    <p class="muted">{t('setup.trustNote')}</p>
    <div class="row">
      <button class="btn primary" onclick={() => onChoose('trust')}>{t('setup.trust')}</button>
      <button class="btn" onclick={() => onChoose('skip')}>{t('setup.skip')}</button>
      <span class="grow"></span>
      <button class="btn" onclick={() => onChoose(null)}>{t('setup.cancel')}</button>
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
    width: min(520px, calc(100vw - 32px));
    padding: 16px 18px;
    border-radius: 10px;
    background: var(--menu-bg);
    border: 1px solid var(--border-strong);
    box-shadow: 0 12px 32px var(--shadow);
    font-size: 13px;
    color: var(--fg);
  }
  h2 {
    margin: 0 0 8px;
    font-size: 14px;
  }
  p {
    margin: 0 0 8px;
    color: var(--fg-dim);
  }
  .muted {
    color: var(--muted);
    font-size: 12px;
  }
  pre {
    margin: 0 0 10px;
    padding: 8px 10px;
    max-height: 160px;
    overflow: auto;
    border-radius: 6px;
    background: var(--bg);
    border: 1px solid var(--border);
    font: 12px var(--mono);
    white-space: pre-wrap;
    word-break: break-all;
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
  .btn:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .btn.primary {
    background: var(--accent-bg);
    color: var(--fg);
  }
</style>
