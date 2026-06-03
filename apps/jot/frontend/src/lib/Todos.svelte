<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Todo, type TodoList } from './api';

  type Props = { todos: Todo[]; initialListId?: number | null };
  let { todos = $bindable([]), initialListId = null }: Props = $props();

  let lists = $state<TodoList[]>([]);
  let activeListId = $state<number | null>(null);
  let loaded = $state(false);
  let listsLoaded = $state(false);

  let draft = $state('');
  let addingList = $state(false);
  let newListTitle = $state('');
  let editingListId = $state<number | null>(null);
  let editListTitle = $state('');

  let addInputRef = $state<HTMLInputElement | null>(null);
  let editInputRef = $state<HTMLInputElement | null>(null);

  const activeList = $derived(lists.find((l) => l.id === activeListId) ?? null);
  const open = $derived(todos.filter((t) => !t.done));
  const done = $derived(todos.filter((t) => t.done));
  const total = $derived(todos.length);
  const pct = $derived(total === 0 ? 0 : Math.round((done.length / total) * 100));

  async function loadLists() {
    try {
      lists = await api.listTodoLists();
      if (lists.length === 0) {
        activeListId = null;
        return;
      }
      const match = initialListId != null && lists.some((l) => l.id === initialListId);
      activeListId = match ? initialListId : lists[0].id;
    } finally {
      listsLoaded = true;
    }
  }

  async function loadTodos() {
    if (activeListId === null) {
      todos = [];
      loaded = true;
      return;
    }
    try {
      todos = await api.listTodos(activeListId);
    } finally {
      loaded = true;
    }
  }

  $effect(() => {
    activeListId;
    loaded = false;
    loadTodos();
  });

  $effect(() => {
    if (activeListId != null) {
      const url = new URL(location.href);
      url.searchParams.set('list', String(activeListId));
      history.replaceState(null, '', url);
    }
  });

  $effect(() => {
    if (addingList && addInputRef) {
      addInputRef.focus();
    }
  });

  $effect(() => {
    if (editingListId !== null && editInputRef) {
      editInputRef.focus();
    }
  });

  async function commitAddList() {
    const title = newListTitle.trim();
    if (!title) {
      addingList = false;
      return;
    }
    const list = await api.createTodoList(title);
    lists = [list, ...lists];
    activeListId = list.id;
    addingList = false;
    newListTitle = '';
  }

  function cancelAddList() {
    addingList = false;
    newListTitle = '';
  }

  function onAddListKey(e: KeyboardEvent) {
    if (e.key === 'Enter') commitAddList();
    if (e.key === 'Escape') cancelAddList();
  }

  function startRenameList(list: TodoList) {
    editingListId = list.id;
    editListTitle = list.title;
  }

  async function commitRenameList() {
    if (editingListId === null) return;
    const title = editListTitle.trim();
    if (!title) {
      editingListId = null;
      return;
    }
    const updated = await api.updateTodoList(editingListId, title);
    lists = lists.map((l) => (l.id === updated.id ? updated : l));
    editingListId = null;
  }

  function cancelRenameList() {
    editingListId = null;
  }

  function onRenameKey(e: KeyboardEvent) {
    if (e.key === 'Enter') commitRenameList();
    if (e.key === 'Escape') cancelRenameList();
  }

  async function deleteList(id: number) {
    if (!confirm('delete this list and all its todos?')) return;
    await api.deleteTodoList(id);
    lists = lists.filter((l) => l.id !== id);
    if (activeListId === id) {
      activeListId = lists.length > 0 ? lists[0].id : null;
    }
  }

  async function addTodo() {
    const text = draft.trim();
    if (!text || activeListId === null) return;
    draft = '';
    const t = await api.createTodo(text, activeListId);
    todos = [t, ...todos];
  }

  async function toggle(t: Todo) {
    const updated = await api.patchTodo(t.id, { done: !t.done });
    todos = todos.map((x) => (x.id === t.id ? updated : x));
  }

  async function del(t: Todo) {
    await api.deleteTodo(t.id);
    todos = todos.filter((x) => x.id !== t.id);
  }

  function onTodoKey(e: KeyboardEvent) {
    if (e.key === 'Enter') addTodo();
  }

  function linkifyText(text: string): Array<{ type: 'text' | 'url'; content: string }> {
    const urlRegex = /(https?:\/\/[^\s]+)/g;
    const parts: Array<{ type: 'text' | 'url'; content: string }> = [];
    let lastIndex = 0;
    let match;

    while ((match = urlRegex.exec(text)) !== null) {
      if (match.index > lastIndex) {
        parts.push({ type: 'text', content: text.slice(lastIndex, match.index) });
      }
      parts.push({ type: 'url', content: match[0] });
      lastIndex = match.index + match[0].length;
    }

    if (lastIndex < text.length) {
      parts.push({ type: 'text', content: text.slice(lastIndex) });
    }

    return parts.length === 0 ? [{ type: 'text', content: text }] : parts;
  }

  onMount(() => {
    loadLists();
  });
</script>

<section class="todos-page">
  {#if !listsLoaded}
    <p class="empty">loading…</p>
  {:else}
    <div class="list-bar">
      {#each lists as list (list.id)}
        {#if editingListId === list.id}
          <input
            class="list-input"
            bind:this={editInputRef}
            bind:value={editListTitle}
            onkeydown={onRenameKey}
            onblur={commitRenameList}
          />
        {:else}
          <button
            class="list-chip"
            class:active={activeListId === list.id}
            onclick={() => (activeListId = list.id)}
            ondblclick={() => startRenameList(list)}
          >
            {list.title || '(untitled)'}
          </button>
        {/if}
      {/each}

      {#if addingList}
        <input
          class="list-input"
          bind:this={addInputRef}
          placeholder="list name…"
          bind:value={newListTitle}
          onkeydown={onAddListKey}
          onblur={() => {
            const title = newListTitle.trim();
            if (title) commitAddList();
            else cancelAddList();
          }}
        />
      {:else}
        <button class="list-chip list-add" onclick={() => (addingList = true)} aria-label="new list">+</button>
      {/if}

      {#if activeListId !== null}
        <div class="list-actions">
          <button
            class="icon-btn"
            onclick={() => {
              const list = lists.find((l) => l.id === activeListId);
              if (list) startRenameList(list);
            }}
            aria-label="rename">✎</button
          >
          <button class="icon-btn del" onclick={() => deleteList(activeListId)} aria-label="delete">✕</button>
        </div>
      {/if}
    </div>

    {#if lists.length === 0}
      <p class="empty">no lists yet — tap + above to create one</p>
    {:else}
      <div class="todo-hero" style="--p: {pct}%">
        <span class="hero-label">{activeList?.title || ''} · open · of · {total}</span>
        <span class="hero-count">{open.length}<span class="total">/{total}</span></span>
        <div class="progress"></div>
      </div>

      <div class="todo-add">
        <input placeholder="new task…" bind:value={draft} onkeydown={onTodoKey} />
        <button class="primary" onclick={addTodo}>add</button>
      </div>

      {#if !loaded}
        <p class="empty">loading…</p>
      {:else if todos.length === 0}
        <p class="empty">nothing yet — add one above</p>
      {:else}
        <div class="todo-group">
          <div class="todo-group-head">
            <span>open</span>
            <span class="k">{open.length}</span>
          </div>
          {#if open.length === 0}
            <p class="empty">all done — nice.</p>
          {:else}
            <ul class="todo-list">
              {#each open as t (t.id)}
                <li>
                  <button class="check" onclick={() => toggle(t)} aria-label="complete"></button>
                  <span class="text">
                    {#each linkifyText(t.text) as part}
                      {#if part.type === 'url'}
                        <a href={part.content} target="_blank" rel="noopener noreferrer">{part.content}</a>
                      {:else}
                        {part.content}
                      {/if}
                    {/each}
                  </span>
                  <button class="del" onclick={() => del(t)} aria-label="delete">✕</button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>

        {#if done.length > 0}
          <div class="todo-group">
            <div class="todo-group-head">
              <span>done</span>
              <span class="k">{done.length}</span>
            </div>
            <ul class="todo-list">
              {#each done as t (t.id)}
                <li class="done">
                  <button class="check on" onclick={() => toggle(t)} aria-label="uncomplete"></button>
                  <span class="text">
                    {#each linkifyText(t.text) as part}
                      {#if part.type === 'url'}
                        <a href={part.content} target="_blank" rel="noopener noreferrer">{part.content}</a>
                      {:else}
                        {part.content}
                      {/if}
                    {/each}
                  </span>
                  <button class="del" onclick={() => del(t)} aria-label="delete">✕</button>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      {/if}
    {/if}
  {/if}
</section>
