<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from './api';
  import { relative } from './time';
  import NoteEditor from './NoteEditor.svelte';

  type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

  type Props = {
    notes: Note[];
    saveStatus: SaveStatus;
  };

  let { notes = $bindable([]), saveStatus = $bindable('idle') }: Props = $props();

  let activeId = $state<number | null>(null);
  let title = $state('');
  let body = $state('');
  let loaded = $state(false);

  // ticks the UI to update "xm ago" labels
  let nowTick = $state(0);

  let dirty = false;
  let suppressNext = false;
  let saveTimer: ReturnType<typeof setTimeout> | null = null;
  let statusTimer: ReturnType<typeof setTimeout> | null = null;
  let inFlight = false;

  const activeNote = $derived(notes.find((n) => n.id === activeId) ?? null);

  $effect(() => {
    // edits → schedule save
    title;
    body;
    if (activeId !== null && !suppressNext) {
      scheduleSave();
    }
  });

  $effect(() => {
    // mobile: reflect editing state on body for CSS layout swap
    document.body.classList.toggle('jot-editing', activeId !== null);
    return () => document.body.classList.remove('jot-editing');
  });

  function scheduleSave() {
    dirty = true;
    saveStatus = 'saving';
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(() => void flush(), 500);
  }

  async function flush() {
    if (!dirty || activeId === null || inFlight) return;
    inFlight = true;
    dirty = false;
    const id = activeId;
    const snapTitle = title;
    const snapBody = body;
    try {
      const updated = await api.updateNote(id, snapTitle, snapBody);
      notes = notes
        .map((n) => (n.id === id ? updated : n))
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
      saveStatus = 'saved';
      if (statusTimer) clearTimeout(statusTimer);
      statusTimer = setTimeout(() => (saveStatus = 'idle'), 1500);
    } catch {
      saveStatus = 'error';
      dirty = true;
    } finally {
      inFlight = false;
    }
  }

  async function loadList() {
    try {
      notes = await api.listNotes();
    } finally {
      loaded = true;
    }
  }

  async function open(n: Note) {
    await flush();
    suppressNext = true;
    activeId = n.id;
    title = n.title;
    body = n.body;
    queueMicrotask(() => (suppressNext = false));
  }

  async function closeEditor() {
    await flush();
    activeId = null;
    title = '';
    body = '';
  }

  async function newNote() {
    await flush();
    const created = await api.createNote('', '');
    notes = [created, ...notes];
    suppressNext = true;
    activeId = created.id;
    title = '';
    body = '';
    queueMicrotask(() => (suppressNext = false));
  }

  async function del() {
    if (activeId === null) return;
    if (!confirm('delete this note?')) return;
    const id = activeId;
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = null;
    }
    dirty = false;
    await api.deleteNote(id);
    notes = notes.filter((n) => n.id !== id);
    activeId = null;
    title = '';
    body = '';
    saveStatus = 'idle';
  }

  function onVisibility() {
    if (document.visibilityState === 'hidden') void flush();
  }

  function onPageHide() {
    if (!dirty || activeId === null) return;
    try {
      fetch(`/api/notes/${activeId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title, body }),
        keepalive: true
      });
    } catch {}
  }

  function preview(n: Note): string {
    const s = (n.body || '').replace(/\s+/g, ' ').trim();
    if (!s) return '';
    return s.length > 160 ? s.slice(0, 160) + '…' : s;
  }

  onMount(() => {
    loadList();
    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pagehide', onPageHide);
    const tick = setInterval(() => (nowTick = Date.now()), 30_000);
    return () => clearInterval(tick);
  });

  onDestroy(() => {
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('pagehide', onPageHide);
    if (saveTimer) clearTimeout(saveTimer);
    if (statusTimer) clearTimeout(statusTimer);
    document.body.classList.remove('jot-editing');
  });

  // re-read nowTick to keep relative() reactive
  function rel(iso: string) {
    nowTick;
    return relative(iso);
  }
</script>

<section class="notes-page">
  <!-- LIST -->
  <div class="list-pane">
    <div class="pane-head">
      <span class="label">
        <span>▎ notes</span>
        <span class="count">{notes.length}</span>
      </span>
      <button class="ghost" onclick={newNote}>＋ new</button>
    </div>

    {#if !loaded}
      <p class="empty">loading…</p>
    {:else if notes.length === 0}
      <p class="empty">no notes yet — tap new</p>
    {:else}
      <ul class="note-list">
        {#each notes as n (n.id)}
          <li>
            <button type="button" class="note-card" class:active={activeId === n.id} onclick={() => open(n)}>
              <span class="title" class:empty={!n.title}>{n.title || '(untitled)'}</span>
              <span class="time">{rel(n.updated_at)}</span>
              <span class="preview" class:empty={!n.body}>{preview(n) || 'empty'}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <!-- EDITOR -->
  {#if activeNote}
    <div class="editor-pane">
      <div class="editor-head">
        <button class="ghost back" onclick={closeEditor}>← back</button>
        <span class="meta">
          <span>id·{activeNote.id}</span>
          <span class="stamp">updated {rel(activeNote.updated_at)}</span>
        </span>
        <button class="danger" onclick={del}>delete</button>
      </div>
      <input class="title-input" placeholder="untitled" bind:value={title} />
      <NoteEditor bind:value={body} placeholder="start writing…" />
    </div>
  {:else}
    <div class="editor-pane">
      <div class="editor-empty">
        <pre class="art">    ┌────────────────┐
    │  <span class="hl">jot</span>  :: ready   │
    │                │
    │    no entry    │
    │    selected    │
    └────────────────┘</pre>
        <div class="hint">
          pick a note from the left — or press <kbd>＋ new</kbd> to start one.
        </div>
      </div>
    </div>
  {/if}
</section>

<!-- mobile FAB for new note (hidden when editor open) -->
{#if activeId === null}
  <button class="fab" onclick={newNote} aria-label="new note">＋</button>
{/if}
