<script lang="ts">
  // The overview's icon: four tiles, drawn (a ▦ glyph sits off the text
  // baseline and changes with the font).
  let { size = 14 }: { size?: number } = $props()
</script>

<svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
  <rect x="1.5" y="1.5" width="5.5" height="5.5" rx="1.5" />
  <rect x="9" y="1.5" width="5.5" height="5.5" rx="1.5" />
  <rect x="1.5" y="9" width="5.5" height="5.5" rx="1.5" />
  <rect x="9" y="9" width="5.5" height="5.5" rx="1.5" />
</svg>

<style>
  svg {
    display: block;
    flex: none;
  }
</style>
