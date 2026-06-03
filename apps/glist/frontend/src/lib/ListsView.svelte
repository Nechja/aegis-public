<script lang="ts">
  import { relative } from './time';
  import type { List, ListStatus } from './api';

  type Props = {
    lists: List[];
    loaded: boolean;
    status: ListStatus;
    onopen: (id: number) => void;
    onClone: (id: number) => void;
    onNewFromTemplate: (id: number) => void;
  };
  let { lists, loaded, status, onopen, onClone, onNewFromTemplate }: Props = $props();

  let search = $state('');
  let tagFilter = $state<string | null>(null);

  const allTags = $derived(
    Array.from(new Set(lists.flatMap((l) => l.tags ?? []))).sort()
  );

  const filtered = $derived(
    lists.filter((l) => {
      if (tagFilter && !(l.tags ?? []).includes(tagFilter)) return false;
      if (!search.trim()) return true;
      const q = search.toLowerCase();
      if (l.name.toLowerCase().includes(q)) return true;
      return (l.items ?? []).some((it) => it.text.toLowerCase().includes(q));
    })
  );

  const heroLabel = $derived(
    status === 'active' ? 'saved · lists' : status === 'archived' ? 'archive · past shops' : 'templates · reusable'
  );

  const emptyMsg = $derived(
    status === 'active'
      ? 'no lists yet — tap ＋ new, or paste a markdown list'
      : status === 'archived'
      ? 'no archived lists — complete or archive a list to see it here'
      : 'no templates yet — save any list as a template to reuse its items'
  );
</script>

<section class="lists-page">
  <div class="hero">
    <div>
      <div class="hero-label">{heroLabel}</div>
      <div class="hero-big">{lists.length}<span class="dim">/total</span></div>
    </div>
    <div class="hero-hint">
      {#if status === 'active'}all lists are saved — reset or duplicate anytime{:else if status === 'archived'}clone back to make a new active list{:else}create a new list from any template{/if}
    </div>
  </div>

  <div class="search">
    <span class="prefix">/</span>
    <input placeholder="search lists and items…" bind:value={search} />
  </div>

  {#if allTags.length > 0}
    <div class="tag-filter-row">
      <button class="tag-chip" class:active={tagFilter === null} onclick={() => (tagFilter = null)}>all</button>
      {#each allTags as t}
        <button class="tag-chip" class:active={tagFilter === t} onclick={() => (tagFilter = tagFilter === t ? null : t)}>
          #{t}
        </button>
      {/each}
    </div>
  {/if}

  {#if !loaded}
    <p class="empty">loading…</p>
  {:else if lists.length === 0}
    <p class="empty">{emptyMsg}</p>
  {:else if filtered.length === 0}
    <p class="empty">no matches</p>
  {:else}
    <ul class="list-grid">
      {#each filtered as l (l.id)}
        <li>
          <button type="button" class="list-card" onclick={() => onopen(l.id)} data-status={status}>
            <span class="list-name">{l.name || '(untitled)'}</span>
            <span class="list-count">{l.item_count ?? l.items?.length ?? 0}</span>
            <span class="list-stamp">
              {#if status === 'archived' && l.completed_at}
                completed {relative(l.completed_at)}
              {:else}
                updated {relative(l.updated_at)}
              {/if}
            </span>
            {#if (l.tags ?? []).length > 0}
              <span class="list-tags">
                {#each l.tags as t}
                  <span class="list-tag">#{t}</span>
                {/each}
              </span>
            {/if}
          </button>
          {#if status === 'archived'}
            <button class="card-action" onclick={() => onClone(l.id)} title="clone to new active list">⎘ clone</button>
          {:else if status === 'template'}
            <button class="card-action primary" onclick={() => onNewFromTemplate(l.id)}>＋ new from</button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>
