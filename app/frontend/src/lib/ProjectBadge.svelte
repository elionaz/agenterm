<script lang="ts">
  // A project's badge: its initials (or emoji) on its color.
  import type { Project } from './api'
  import { badgeContent, projectColor } from './badge'

  let { project, size = 16 }: { project: Pick<Project, 'id' | 'name' | 'color' | 'icon'>; size?: number } = $props()

  const content = $derived(badgeContent(project))
  const color = $derived(projectColor(project))
</script>

<span
  class="badge"
  class:emoji={content.emoji}
  style:--c="var(--{color})"
  style:width="{size}px"
  style:height="{size}px"
  style:font-size="{Math.round(size * (content.emoji ? 0.72 : 0.5))}px"
  aria-hidden="true">{content.text}</span
>

<style>
  .badge {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    border-radius: 4px;
    background: var(--c);
    color: var(--badge-fg);
    font-weight: 700;
    line-height: 1;
    letter-spacing: -0.02em;
    white-space: nowrap;
    user-select: none;
  }
  /* An emoji keeps its own colors: a soft tint and a ring of the color. */
  .badge.emoji {
    background: color-mix(in srgb, var(--c) 22%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--c) 55%, transparent);
    font-weight: 400;
    letter-spacing: 0;
  }
</style>
