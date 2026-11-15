<script lang="ts">
  import { attentionLabel, needsAttention, type Profile, type Project, type Session } from './api'

  interface Props {
    projects: Project[]
    sessions: Session[]
    profiles: Profile[]
    selectedId: string | null
    currentProjectId: string | null
    onSelect: (id: string) => void
    onSelectProject: (id: string) => void
    onSpawn: (projectId: string, profileId: string) => void
    onKill: (id: string) => void
    onAddProject: () => void
    onRemoveProject: (id: string) => void
    onOpenSettings: () => void
  }

  let {
    projects,
    sessions,
    profiles,
    selectedId,
    currentProjectId,
    onSelect,
    onSelectProject,
    onSpawn,
    onKill,
    onAddProject,
    onRemoveProject,
    onOpenSettings,
  }: Props = $props()

  let collapsed = $state<Record<string, boolean>>({})
  let menuFor = $state<string | null>(null)

  const byProject = $derived.by(() => {
    const m = new Map<string, Session[]>()
    for (const s of sessions) {
      const list = m.get(s.projectId) ?? []
      list.push(s)
      m.set(s.projectId, list)
    }
    return m
  })

  function stateLabel(s: Session): string {
    if (needsAttention(s)) return attentionLabel(s.attentionKind)
    switch (s.state) {
      case 'running':
        return 'trabajando'
      case 'waiting_input':
        return 'esperando'
      case 'idle':
        return 'inactivo'
      case 'exited':
        return s.exitCode != null ? `exit ${s.exitCode}` : 'terminado'
      default:
        return s.state
    }
  }

  function badgeClass(s: Session): string {
    if (s.state === 'exited') return s.exitCode ? 'exited fail' : 'exited'
    if (needsAttention(s)) return `${s.state} attention ${s.attentionKind}`
    return s.state
  }

  function tooltip(s: Session): string {
    const base = `${s.cmd} · ${s.cwd} · ${s.id}`
    if (!s.attentionKind || s.state === 'exited') return base
    const what = attentionLabel(s.attentionKind)
    return `${what}${s.attentionMessage ? `: ${s.attentionMessage}` : ''}\n${base}`
  }

  function toggleMenu(projectId: string, ev: MouseEvent) {
    ev.stopPropagation()
    menuFor = menuFor === projectId ? null : projectId
  }

  function pick(projectId: string, profileId: string) {
    menuFor = null
    onSpawn(projectId, profileId)
  }

  function closeMenu() {
    menuFor = null
  }
</script>

<svelte:window onclick={closeMenu} onkeydown={(e) => e.key === 'Escape' && closeMenu()} />

<aside class="sidebar">
  <div class="titlebar drag">
    <span class="brand">agenterm</span>
  </div>

  <div class="projects">
    {#each projects as p (p.id)}
      {@const list = byProject.get(p.id) ?? []}
      {@const pending = list.filter(needsAttention).length}
      <section class="project" class:current={p.id === currentProjectId}>
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="project-row" title={p.path} onclick={() => onSelectProject(p.id)}>
          <button
            class="chevron"
            class:closed={collapsed[p.id]}
            aria-label={collapsed[p.id] ? 'Expandir' : 'Contraer'}
            onclick={(e) => {
              e.stopPropagation()
              collapsed[p.id] = !collapsed[p.id]
            }}>▾</button
          >
          <span class="project-name">{p.name}</span>
          {#if pending}
            <span class="attn-count" title={`${pending} ${pending === 1 ? 'sesión necesita' : 'sesiones necesitan'} tu atención`}
              >{pending}</span
            >
          {:else if list.length}<span class="count">{list.length}</span>{/if}
          <span class="row-actions">
            <button class="icon" title="Quitar proyecto (no borra archivos)" onclick={(e) => {
              e.stopPropagation()
              onRemoveProject(p.id)
            }}>−</button>
            <button class="icon" title="Nueva sesión" onclick={(e) => toggleMenu(p.id, e)}>+</button>
          </span>
          {#if menuFor === p.id}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div class="menu" onclick={(e) => e.stopPropagation()}>
              {#each profiles as prof (prof.id)}
                <button class="menu-item" title={prof.path ?? prof.cmd} onclick={() => pick(p.id, prof.id)}>
                  <span>{prof.name || prof.id}</span>
                  <span class="menu-cmd">{prof.id}</span>
                </button>
              {:else}
                <div class="menu-empty">Sin agentes disponibles</div>
              {/each}
              <div class="menu-sep"></div>
              <button
                class="menu-item muted"
                onclick={() => {
                  menuFor = null
                  onOpenSettings()
                }}><span>Elegir agentes…</span><span class="menu-cmd">⌘,</span></button
              >
            </div>
          {/if}
        </div>

        {#if !collapsed[p.id]}
          <ul class="sessions">
            {#each list as s, i (s.id)}
              <li>
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <!-- svelte-ignore a11y_no_static_element_interactions -->
                <div
                  class="session"
                  class:selected={s.id === selectedId}
                  class:needs={needsAttention(s)}
                  title={tooltip(s)}
                  onclick={() => onSelect(s.id)}
                >
                  <span class="dot {badgeClass(s)}"></span>
                  <span class="session-name">{s.profileId}</span>
                  <span class="state {badgeClass(s)}">{stateLabel(s)}</span>
                  {#if i < 9 && p.id === currentProjectId}<span class="kbd">⌘{i + 1}</span>{/if}
                  <button
                    class="icon kill"
                    title={s.state === 'exited' ? 'Quitar de la lista' : 'Terminar sesión'}
                    onclick={(e) => {
                      e.stopPropagation()
                      onKill(s.id)
                    }}>×</button
                  >
                </div>
              </li>
            {:else}
              <li class="no-sessions">Sin sesiones</li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}
  </div>

  <div class="footer">
    <button class="add-project" onclick={onAddProject}>+ Proyecto</button>
    <button class="gear" title="Ajustes (⌘,)" aria-label="Ajustes" onclick={onOpenSettings}>⚙</button>
  </div>
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    width: 248px;
    min-width: 248px;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border);
    user-select: none;
  }
  .titlebar {
    height: 38px;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    padding: 0 12px 0 80px; /* room for the traffic lights */
  }
  .brand {
    font-size: 11px;
    letter-spacing: 0.08em;
    color: var(--muted);
    text-transform: lowercase;
  }
  .projects {
    flex: 1;
    overflow-y: auto;
    padding: 4px 6px 8px;
  }
  .project + .project {
    margin-top: 4px;
  }
  .project-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
    height: 28px;
    padding: 0 4px;
    border-radius: 6px;
    cursor: default;
    color: var(--fg-dim);
    font-weight: 600;
    font-size: 12px;
  }
  .project-row:hover {
    background: var(--hover);
  }
  .project.current > .project-row {
    color: var(--fg);
  }
  .chevron {
    width: 16px;
    color: var(--muted);
    font-size: 10px;
    transition: transform 0.12s;
  }
  .chevron.closed {
    transform: rotate(-90deg);
  }
  .project-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .count {
    color: var(--muted);
    font-weight: 400;
    font-size: 11px;
  }
  .row-actions {
    display: flex;
    gap: 2px;
  }
  .row-actions .icon:first-child {
    visibility: hidden;
  }
  .project-row:hover .row-actions .icon:first-child {
    visibility: visible;
  }
  .icon {
    width: 20px;
    height: 20px;
    border-radius: 4px;
    color: var(--muted);
    font-size: 15px;
    line-height: 1;
  }
  .icon:hover {
    background: var(--hover-strong);
    color: var(--fg);
  }
  .menu {
    position: absolute;
    top: 28px;
    right: 4px;
    z-index: 10;
    min-width: 180px;
    padding: 4px;
    background: var(--menu-bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
    font-weight: 400;
  }
  .menu-item {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    padding: 6px 8px;
    border-radius: 5px;
    color: var(--fg);
    text-align: left;
  }
  .menu-item:hover {
    background: var(--accent-bg);
  }
  .menu-cmd {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11px;
  }
  .menu-empty {
    padding: 6px 8px;
    color: var(--muted);
  }
  .sessions {
    list-style: none;
    margin: 0;
    padding: 0 0 0 10px;
  }
  .session {
    display: flex;
    align-items: center;
    gap: 7px;
    height: 26px;
    padding: 0 4px 0 8px;
    border-radius: 6px;
    color: var(--fg-dim);
    cursor: default;
  }
  .session:hover {
    background: var(--hover);
  }
  .session.selected {
    background: var(--accent-bg);
    color: var(--fg);
  }
  .session-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    font-size: 10.5px;
    color: var(--muted);
  }
  .state.waiting_input,
  .state.attention {
    color: var(--amber);
    font-weight: 600;
  }
  .state.attention.done {
    color: var(--green);
  }
  .state.attention.error {
    color: var(--red);
  }
  .session.needs {
    color: var(--fg);
    background: color-mix(in srgb, var(--amber) 9%, transparent);
    box-shadow: inset 2px 0 0 var(--amber);
  }
  .session.needs.selected {
    background: var(--accent-bg);
  }
  .attn-count {
    min-width: 16px;
    height: 16px;
    padding: 0 5px;
    border-radius: 8px;
    background: var(--amber);
    color: #1d1f24;
    font-size: 10.5px;
    font-weight: 700;
    line-height: 16px;
    text-align: center;
  }
  .state.exited.fail {
    color: var(--red);
  }
  .kbd {
    font-size: 10px;
    color: var(--muted);
    opacity: 0.7;
  }
  .session .kill {
    display: none;
  }
  .session:hover .kill {
    display: inline-block;
  }
  .session:hover .kbd {
    display: none;
  }
  .dot {
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--grey);
  }
  .dot.running {
    background: var(--blue);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .dot.waiting_input {
    background: var(--amber);
    box-shadow: 0 0 6px var(--amber);
  }
  .dot.idle {
    background: var(--green);
  }
  .dot.attention {
    background: var(--amber);
    animation: attn 1.4s ease-in-out infinite;
  }
  .dot.attention.done {
    background: var(--green);
    animation: none;
    box-shadow: 0 0 6px var(--green);
  }
  .dot.attention.error {
    background: var(--red);
    animation: none;
    box-shadow: 0 0 6px var(--red);
  }
  @keyframes attn {
    0%,
    100% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--amber) 70%, transparent);
    }
    50% {
      box-shadow: 0 0 0 4px color-mix(in srgb, var(--amber) 0%, transparent);
    }
  }
  .dot.exited {
    background: var(--grey);
  }
  .dot.exited.fail {
    background: var(--red);
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
      box-shadow: 0 0 0 0 rgba(97, 175, 239, 0.6);
    }
    50% {
      opacity: 0.55;
      box-shadow: 0 0 0 3px rgba(97, 175, 239, 0);
    }
  }
  .no-sessions {
    padding: 4px 8px 6px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .footer {
    flex: none;
    display: flex;
    gap: 6px;
    padding: 8px;
    border-top: 1px solid var(--border);
  }
  .gear {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 6px;
    color: var(--muted);
    font-size: 15px;
  }
  .gear:hover {
    color: var(--fg);
    background: var(--hover);
  }
  .menu-sep {
    height: 1px;
    margin: 4px 2px;
    background: var(--border-strong);
  }
  .menu-item.muted {
    color: var(--fg-dim);
  }
  .add-project {
    flex: 1;
    height: 28px;
    border-radius: 6px;
    color: var(--fg-dim);
    border: 1px dashed var(--border-strong);
  }
  .add-project:hover {
    color: var(--fg);
    background: var(--hover);
  }
</style>
