<script lang="ts">
  import { progressLabel } from './activity'
  import type { Progress } from './api'
  import { t } from './i18n.svelte'

  let { progress, thick = false }: { progress: Progress; thick?: boolean } = $props()
</script>

<div class="bar {progress.state}" class:thick title={t('progress.title', { label: progressLabel(progress) })}>
  <div class="fill" style:width={progress.state === 'indeterminate' ? undefined : `${progress.value}%`}></div>
</div>

<style>
  .bar {
    position: relative;
    height: 2px;
    overflow: hidden;
    border-radius: 1px;
    background: color-mix(in srgb, var(--blue) 15%, transparent);
  }
  .bar.thick {
    height: 3px;
  }
  .fill {
    height: 100%;
    background: var(--blue);
    transition: width 0.3s ease;
  }
  .error .fill {
    background: var(--red);
  }
  .error {
    background: color-mix(in srgb, var(--red) 15%, transparent);
  }
  .paused .fill {
    background: var(--amber);
  }
  .indeterminate .fill {
    position: absolute;
    width: 30%;
    animation: slide 1.2s ease-in-out infinite;
  }
  @keyframes slide {
    from {
      left: -30%;
    }
    to {
      left: 100%;
    }
  }
</style>
