<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import Notes from './lib/Notes.svelte';
  import Todos from './lib/Todos.svelte';
  import type { Note, Todo } from './lib/api';
  import { clockHHMMSS } from './lib/time';

  type Tab = 'notes' | 'todos';

  function readTab(): Tab {
    const params = new URLSearchParams(location.search);
    if (params.has('list')) return 'todos';
    const v = localStorage.getItem('jot.tab');
    return v === 'todos' ? 'todos' : 'notes';
  }

  function readInitialListId(): number | null {
    const params = new URLSearchParams(location.search);
    const raw = params.get('list');
    if (!raw) return null;
    const n = parseInt(raw, 10);
    return isNaN(n) ? null : n;
  }

  let tab = $state<Tab>(readTab());
  let notes = $state<Note[]>([]);
  let todos = $state<Todo[]>([]);
  let clock = $state(clockHHMMSS());
  let tick: ReturnType<typeof setInterval> | null = null;
  let saveStatus = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');

  const openTodos = $derived(todos.filter((t) => !t.done).length);

  function setTab(t: Tab) {
    tab = t;
    localStorage.setItem('jot.tab', t);
  }

  onMount(() => {
    tick = setInterval(() => (clock = clockHHMMSS()), 1000);
  });

  onDestroy(() => {
    if (tick) clearInterval(tick);
  });
</script>

<header class="app-head">
  <div class="brand">
    <span class="prompt">~/</span>
    <span class="name">jot</span>
    <span class="cursor"></span>
  </div>

  <nav class="tabs">
    <button class:active={tab === 'notes'} onclick={() => setTab('notes')}>
      notes<span class="count">{notes.length}</span>
    </button>
    <button class:active={tab === 'todos'} onclick={() => setTab('todos')}>
      todos<span class="count">{openTodos}</span>
    </button>
  </nav>

  <div class="head-right">
    <span class="pill">online</span>
  </div>
</header>

<main class="surface">
  {#if tab === 'notes'}
    <Notes bind:notes bind:saveStatus />
  {:else}
    <Todos bind:todos initialListId={readInitialListId()} />
  {/if}
</main>

<footer class="status-bar">
  <span><kbd>⌘</kbd><kbd>N</kbd> new</span>
  <span><kbd>esc</kbd> close</span>
  <span class="grow"></span>
  {#if tab === 'notes'}
    <span class="status {saveStatus}">
      {#if saveStatus === 'saving'}saving{:else if saveStatus === 'saved'}saved{:else if saveStatus === 'error'}error{:else}idle{/if}
    </span>
  {/if}
  <span class="clock">{clock}</span>
</footer>
