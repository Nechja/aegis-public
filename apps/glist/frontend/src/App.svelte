<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type List, type ListStatus } from './lib/api';
  import { route, navigate } from './lib/route.svelte';
  import ListsView from './lib/ListsView.svelte';
  import Detail from './lib/Detail.svelte';
  import Import from './lib/Import.svelte';
  import Toasts from './lib/Toasts.svelte';
  import Celebrate from './lib/Celebrate.svelte';
  import { toast } from './lib/toasts.svelte';

  let lists = $state<List[]>([]);
  let loaded = $state(false);
  let showImport = $state(false);

  const currentStatus = $derived(route.current.name === 'lists' ? route.current.status : 'active');

  async function load() {
    loaded = false;
    try {
      lists = await api.listLists(currentStatus);
    } finally {
      loaded = true;
    }
  }

  async function createNew() {
    const name = prompt('name your list:', 'new list');
    if (!name) return;
    const l = await api.createList(name);
    navigate({ name: 'detail', id: l.id });
  }

  function onImportDone(l: List) {
    showImport = false;
    navigate({ name: 'detail', id: l.id });
  }

  async function onListDelete(id: number) {
    lists = lists.filter((l) => l.id !== id);
    if (route.current.name === 'detail' && route.current.id === id) {
      navigate({ name: 'lists', status: 'active' });
    }
  }

  async function cloneFromArchive(id: number) {
    const l = await api.duplicateList(id, 'copy', 'active');
    toast('cloned to active', { tone: 'spark', ttl: 2500 });
    navigate({ name: 'detail', id: l.id });
  }

  async function newFromTemplate(id: number) {
    const l = await api.duplicateList(id, 'new shop', 'active');
    toast('new list from template', { tone: 'spark', ttl: 2500 });
    navigate({ name: 'detail', id: l.id });
  }

  function goTo(status: ListStatus) {
    navigate({ name: 'lists', status });
  }

  function goHome() {
    goTo('active');
  }

  // reload list collection whenever the status view changes
  $effect(() => {
    if (route.current.name === 'lists') {
      void load();
    }
  });

  onMount(() => {
    if (route.current.name === 'lists') void load();
  });
</script>

<header class="app-head">
  <button type="button" class="brand-btn" onclick={goHome} aria-label="go home">
    <span class="brand">
      <span class="prompt">~/</span>
      <span class="name">g<span class="slash">/</span>list</span>
      <span class="cursor"></span>
    </span>
  </button>

  {#if route.current.name === 'lists'}
    <nav class="tabs">
      <button class:active={currentStatus === 'active'} onclick={() => goTo('active')}>active</button>
      <button class:active={currentStatus === 'archived'} onclick={() => goTo('archived')}>archive</button>
      <button class:active={currentStatus === 'template'} onclick={() => goTo('template')}>templates</button>
    </nav>
  {/if}

  <div class="head-right">
    {#if route.current.name === 'lists' && currentStatus === 'active'}
      <button class="primary" onclick={createNew}>＋ new</button>
      <button class="ghost" onclick={() => (showImport = !showImport)}>
        {showImport ? 'cancel' : 'paste md'}
      </button>
    {:else if route.current.name !== 'lists'}
      <button class="ghost" onclick={goHome}>← lists</button>
    {/if}
  </div>
</header>

<Celebrate />
<Toasts />

<main class="surface">
  {#if route.current.name === 'detail'}
    <Detail id={route.current.id} onback={goHome} onDelete={onListDelete} />
  {:else}
    {#if showImport && currentStatus === 'active'}
      <Import oncreated={onImportDone} oncancel={() => (showImport = false)} />
    {/if}
    <ListsView
      {lists}
      {loaded}
      status={currentStatus}
      onopen={(id) => navigate({ name: 'detail', id })}
      onClone={cloneFromArchive}
      onNewFromTemplate={newFromTemplate}
    />
  {/if}
</main>
