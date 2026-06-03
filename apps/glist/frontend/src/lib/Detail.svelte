<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    api,
    CATEGORIES,
    CATEGORY_ICONS,
    formatQty,
    hasQty,
    isFuzzy,
    type Category,
    type Item,
    type List
  } from './api';
  import { cheer, toast } from './toasts.svelte';
  import { shareLink } from './route.svelte';
  import { relative } from './time';

  type Props = {
    id: number;
    onback: () => void;
    onDelete: (id: number) => void;
  };
  let { id, onback, onDelete }: Props = $props();

  let list = $state<List | null>(null);
  let draft = $state('');
  let editingName = $state(false);
  let nameDraft = $state('');
  let menu = $state(false);
  let fixingAll = $state(false);
  let expandedId = $state<number | null>(null);

  // milestone tracking — tracks the highest percent-done we've already cheered
  // for. Per-list so different lists get their own celebration cycle.
  let lastMilestone = $state(0);
  let firstBoughtSeen = $state(false);

  // cheer-generation state
  let generating = $state(false);

  // true when this list has a personalized cheer pool stored
  const hasListCheers = $derived(!!list && !!list.cheers && Object.keys(list.cheers).length > 0);
  // always pass listId so the server prefers the list pool when it exists
  const cheerOpts = $derived(() => (list ? { listId: list.id } : {}));

  // tag editing
  let tagDraft = $state('');
  let showTagEditor = $state(false);

  // editing count per item
  let editingCountFor = $state<number | null>(null);
  let countDraft = $state('');
  let unitDraft = $state('');

  // replacement popover
  let replaceFor = $state<number | null>(null);
  let replaceLoading = $state(false);
  let replaceError = $state('');
  let replaceSuggestions = $state<{ text: string; reason: string }[]>([]);

  const items = $derived(list?.items ?? []);
  const total = $derived(items.length);
  const boughtList = $derived(items.filter((i) => i.bought));
  const bought = $derived(boughtList.length);
  const missing = $derived(items.filter((i) => i.missing).length);
  const pct = $derived(total === 0 ? 0 : Math.round((bought / total) * 100));
  const otherItems = $derived(items.filter((i) => i.category === 'other'));

  // active = not bought (missing items stay inline with attention styling so
  // the shopper can still find a sub).
  const activeItems = $derived(items.filter((i) => !i.bought));
  const grouped = $derived.by(() => {
    const g = new Map<Category, Item[]>();
    for (const cat of CATEGORIES) g.set(cat, []);
    for (const it of activeItems) g.get(it.category)?.push(it);
    return CATEGORIES
      .map((c) => ({ cat: c, items: g.get(c) ?? [] }))
      .filter((g) => g.items.length > 0);
  });

  const sortedBought = $derived(
    [...boughtList].sort((a, b) => (b.bought_at ?? '').localeCompare(a.bought_at ?? ''))
  );

  let showDone = $state(false);

  // Prime the milestone tracker to the current list state. Prevents re-firing
  // cheers we've already shown. Called whenever list state changes from
  // outside the local optimistic path (load, reset, SSE refetch).
  function primeMilestoneTracker() {
    if (!list) return;
    const cur = list.items ?? [];
    const b = cur.filter((i) => i.bought).length;
    firstBoughtSeen = b > 0;
    const p = cur.length === 0 ? 0 : Math.round((b / cur.length) * 100);
    lastMilestone = p >= 100 ? 100 : p >= 75 ? 75 : p >= 50 ? 50 : p >= 25 ? 25 : 0;
  }

  async function load() {
    list = await api.getList(id);
    primeMilestoneTracker();
  }

  // ---- live updates via SSE ----
  let es: EventSource | null = null;
  let refetchTimer: ReturnType<typeof setTimeout> | null = null;

  function openEvents() {
    closeEvents();
    try {
      es = new EventSource(`/api/lists/${id}/events`);
    } catch {
      return;
    }
    es.addEventListener('update', () => scheduleRefetch());
    es.onerror = () => {
      // EventSource auto-reconnects on transient errors; nothing to do
    };
  }

  function closeEvents() {
    if (es) {
      es.close();
      es = null;
    }
    if (refetchTimer) {
      clearTimeout(refetchTimer);
      refetchTimer = null;
    }
  }

  // debounced refetch: bursts of mutations collapse into one round-trip
  function scheduleRefetch() {
    if (refetchTimer) clearTimeout(refetchTimer);
    refetchTimer = setTimeout(async () => {
      try {
        const fresh = await api.getList(id);
        list = fresh;
        // re-prime from server state so cross-device uncheck then recheck
        // can fire milestones again correctly
        primeMilestoneTracker();
      } catch {}
    }, 250);
  }

  // fire appropriate shoppy-fren cheers after a check flips to bought
  function maybeCheer() {
    if (!list) return;
    const all = list.items ?? [];
    const total = all.length;
    if (total === 0) return;
    const b = all.filter((i) => i.bought).length;
    if (b === 0) return;
    const p = Math.round((b / total) * 100);
    const co = cheerOpts();
    if (b === 1 && !firstBoughtSeen) {
      firstBoughtSeen = true;
      cheer('first', co);
    }
    if (p >= 100 && lastMilestone < 100) {
      lastMilestone = 100;
      cheer('complete', co);
    } else if (p >= 75 && lastMilestone < 75) {
      lastMilestone = 75;
      cheer('milestone75', co);
    } else if (p >= 50 && lastMilestone < 50) {
      lastMilestone = 50;
      cheer('milestone50', co);
    } else if (p >= 25 && lastMilestone < 25) {
      lastMilestone = 25;
      cheer('milestone25', co);
    }
  }

  async function add() {
    const text = draft.trim();
    if (!text || !list) return;
    draft = '';
    const it = await api.createItem(list.id, text);
    list = { ...list, items: [...(list.items ?? []), it] };
    if (it.category === 'other') {
      setTimeout(async () => {
        if (!list) return;
        try {
          list = await api.getList(list.id);
        } catch {}
      }, 15000);
    }
  }

  async function toggle(t: Item) {
    if (!list) return;
    const wasBought = t.bought;
    const updated = await api.patchItem(t.id, {
      bought: !t.bought,
      ...(t.bought ? {} : { missing: false })
    });
    list = { ...list, items: (list.items ?? []).map((x) => (x.id === t.id ? updated : x)) };
    if (!wasBought && updated.bought) {
      // immediate instant feedback — user just tapped a check, show them
      // something happened before the milestone cheer (if any) decides to fire
      toast('✓ in cart', { tone: 'spark', ttl: 1500, source: null, face: '(˃ᴗ˂)' });
      // on first check of this session, auto-expand DONE so they see where
      // items are going
      if (!firstBoughtSeen) showDone = true;
      maybeCheer();
    }
  }

  async function toggleMissing(t: Item) {
    if (!list) return;
    const wasMissing = t.missing;
    const updated = await api.patchItem(t.id, {
      missing: !t.missing,
      ...(t.missing ? {} : { bought: false })
    });
    list = { ...list, items: (list.items ?? []).map((x) => (x.id === t.id ? updated : x)) };
    if (!wasMissing && updated.missing) {
      cheer('missing', cheerOpts());
    }
  }

  async function generateCheers() {
    if (!list || generating) return;
    generating = true;
    menu = false;
    toast("shoppy-fren is learning this list…", {
      tone: 'milestone',
      ttl: 180000,
      source: 'shoppy-fren'
    });
    try {
      const res = await api.generateListCheers(list.id);
      // refetch so list.cheers updates locally
      list = await api.getList(list.id);
      const count = Object.values(res.cheers).reduce((n, arr) => n + arr.length, 0);
      toast(`${count} personalized cheers ready!`, {
        tone: 'cheer',
        ttl: 5000,
        source: 'shoppy-fren'
      });
    } catch {
      toast("couldn't generate — try again later?", {
        tone: 'error',
        ttl: 4000,
        source: 'shoppy-fren'
      });
    } finally {
      generating = false;
    }
  }

  async function setCategory(t: Item, cat: Category) {
    if (!list) return;
    const updated = await api.patchItem(t.id, { category: cat });
    list = { ...list, items: (list.items ?? []).map((x) => (x.id === t.id ? updated : x)) };
  }

  async function reAi(t: Item) {
    const res = await api.aiCategorize(t.text);
    await setCategory(t, res.category);
  }

  async function fixAll() {
    if (!list || otherItems.length === 0 || fixingAll) return;
    fixingAll = true;
    try {
      const batch = otherItems.map((i) => ({ id: i.id, text: i.text }));
      const resolved = await api.aiCategorizeBatch(batch);
      await Promise.all(
        resolved
          .filter((r) => r.category !== 'other')
          .map((r) => api.patchItem(r.id, { category: r.category }))
      );
      list = await api.getList(list.id);
    } finally {
      fixingAll = false;
    }
  }

  async function del(t: Item) {
    if (!list) return;
    await api.deleteItem(t.id);
    list = { ...list, items: (list.items ?? []).filter((x) => x.id !== t.id) };
  }

  async function copyLink() {
    if (!list) return;
    const url = shareLink({ name: 'detail', id: list.id });
    try {
      if (navigator.share) {
        await navigator.share({ title: list.name, url });
      } else {
        await navigator.clipboard.writeText(url);
        toast('link copied', { tone: 'spark', ttl: 2500 });
      }
    } catch {
      // user cancelled, or clipboard denied — quiet
    }
    menu = false;
  }

  async function reset() {
    if (!list || !confirm('uncheck everything?')) return;
    await api.resetList(list.id);
    list = {
      ...list,
      items: (list.items ?? []).map((x) => ({ ...x, bought: false, missing: false, bought_at: null }))
    };
    // milestones can fire again on the next round of shopping
    primeMilestoneTracker();
    menu = false;
  }

  async function duplicate() {
    if (!list) return;
    await api.duplicateList(list.id, list.name + ' copy');
    menu = false;
    onback();
  }

  async function removeList() {
    if (!list) return;
    if (!confirm(`delete list "${list.name}"?`)) return;
    await api.deleteList(list.id);
    onDelete(list.id);
    onback();
  }

  async function archiveList() {
    if (!list) return;
    await api.setListStatus(list.id, 'archived');
    toast('archived', { tone: 'spark', ttl: 2000 });
    menu = false;
    onback();
  }

  async function unarchive() {
    if (!list) return;
    await api.setListStatus(list.id, 'active');
    toast('back to active', { tone: 'spark', ttl: 2000 });
    menu = false;
    onback();
  }

  async function saveAsTemplate() {
    if (!list) return;
    await api.duplicateList(list.id, list.name, 'template');
    toast('saved as template', { tone: 'cheer', ttl: 2500 });
    menu = false;
  }

  async function addTag() {
    const t = tagDraft.trim().toLowerCase().replace(/^#/, '');
    if (!list || !t) return;
    const existing = list.tags ?? [];
    if (existing.includes(t)) {
      tagDraft = '';
      return;
    }
    const next = [...existing, t];
    const updated = await api.setListTags(list.id, next);
    list = { ...list, tags: updated.tags, updated_at: updated.updated_at };
    tagDraft = '';
  }

  async function removeTag(t: string) {
    if (!list) return;
    const next = (list.tags ?? []).filter((x) => x !== t);
    const updated = await api.setListTags(list.id, next);
    list = { ...list, tags: updated.tags, updated_at: updated.updated_at };
  }

  function startRename() {
    if (!list) return;
    nameDraft = list.name;
    editingName = true;
    menu = false;
  }

  async function commitRename() {
    if (!list) return;
    const n = nameDraft.trim();
    editingName = false;
    if (!n || n === list.name) return;
    const updated = await api.renameList(list.id, n);
    list = { ...list, name: updated.name, updated_at: updated.updated_at };
  }

  // count editing
  function startCountEdit(t: Item) {
    editingCountFor = t.id;
    countDraft = formatNum(t.count);
    unitDraft = t.unit;
  }

  async function commitCount(t: Item) {
    const parsed = parseFloat(countDraft.trim());
    const count = Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
    editingCountFor = null;
    if (count === t.count && unitDraft === t.unit) return;
    const updated = await api.patchItem(t.id, { count, unit: unitDraft.trim() });
    if (!list) return;
    list = { ...list, items: (list.items ?? []).map((x) => (x.id === t.id ? updated : x)) };
  }

  function cancelCount() {
    editingCountFor = null;
  }

  function formatNum(n: number): string {
    if (Number.isInteger(n)) return String(n);
    return String(n);
  }

  // replacement popover
  async function openReplace(t: Item) {
    replaceFor = t.id;
    replaceSuggestions = [];
    replaceError = '';
    replaceLoading = true;
    try {
      const res = await api.aiReplace(t.text);
      replaceSuggestions = res.suggestions;
      if (res.suggestions.length === 0) replaceError = 'no suggestions — try rephrasing';
    } catch {
      replaceError = 'failed — try again';
    } finally {
      replaceLoading = false;
    }
  }

  function closeReplace() {
    replaceFor = null;
    replaceSuggestions = [];
    replaceError = '';
  }

  async function applyReplacement(t: Item, newText: string) {
    const updated = await api.patchItem(t.id, { text: newText });
    const cat = await api.aiCategorize(newText);
    const final = await api.patchItem(t.id, { category: cat.category });
    if (!list) return;
    list = {
      ...list,
      items: (list.items ?? []).map((x) => (x.id === t.id ? { ...final, bought: updated.bought } : x))
    };
    closeReplace();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') add();
  }

  onMount(() => {
    load();
    openEvents();
  });

  onDestroy(() => {
    closeEvents();
  });
</script>

<section class="detail">
  <div class="detail-head">
    {#if editingName && list}
      <input
        class="name-edit"
        bind:value={nameDraft}
        onblur={commitRename}
        onkeydown={(e) => e.key === 'Enter' && commitRename()}
      />
    {:else if list}
      <h2 class="detail-name">
        <button type="button" onclick={startRename}>{list.name}</button>
      </h2>
    {/if}
    <div class="menu-wrap">
      <button class="ghost" onclick={() => (menu = !menu)} aria-label="list actions">⋯</button>
      {#if menu}
        <div class="menu">
          <button onclick={copyLink}>⇪ share link</button>
          <button onclick={() => { showTagEditor = true; menu = false; }}># edit tags</button>
          <button onclick={generateCheers} disabled={generating}>
            {#if generating}
              ✨ thinking…
            {:else if hasListCheers}
              ✨ regenerate cheers
            {:else}
              ✨ generate cheers
            {/if}
          </button>
          <button onclick={reset}>↺ uncheck all</button>
          <button onclick={duplicate}>⎘ duplicate</button>
          <button onclick={saveAsTemplate}>✎ save as template</button>
          {#if list && list.status === 'archived'}
            <button onclick={unarchive}>↺ move to active</button>
          {:else if list && list.status === 'active'}
            <button onclick={archiveList}>📦 archive</button>
          {/if}
          <button onclick={startRename}>✎ rename</button>
          <button onclick={removeList} class="danger-text">✕ delete list</button>
        </div>
      {/if}
    </div>
  </div>

  {#if list}
    {#if (list.tags ?? []).length > 0 || showTagEditor}
      <div class="tag-row">
        {#each list.tags ?? [] as t}
          <span class="tag">
            #{t}
            <button class="tag-x" onclick={() => removeTag(t)} aria-label="remove tag">×</button>
          </span>
        {/each}
        {#if showTagEditor}
          <input
            class="tag-input"
            placeholder="tag (e.g. weekly)"
            bind:value={tagDraft}
            onkeydown={(e) => {
              if (e.key === 'Enter') addTag();
              if (e.key === 'Escape') { showTagEditor = false; tagDraft = ''; }
            }}
            onblur={() => { if (tagDraft.trim()) addTag(); else showTagEditor = false; }}
            autofocus
          />
        {:else}
          <button class="tag-add" onclick={() => (showTagEditor = true)}>＋ tag</button>
        {/if}
      </div>
    {:else}
      <button class="tag-add solo" onclick={() => (showTagEditor = true)}>＋ add tag</button>
    {/if}

    <div class="progress">
      <div class="progress-bar" style="--p: {pct}%"></div>
      <span class="progress-label">
        {bought} / {total} bought · {pct}%
        {#if missing > 0}<span class="missing-pill">· {missing} missing</span>{/if}
        {#if hasListCheers}<span class="custom-pill" title="personalized cheers ready">✨ custom</span>{/if}
        {#if list.status === 'archived'}<span class="status-pill">archived</span>{/if}
        {#if list.status === 'template'}<span class="status-pill template">template</span>{/if}
      </span>
    </div>

    <div class="add">
      <span class="add-prefix">＋</span>
      <input placeholder="add an item…" bind:value={draft} onkeydown={onKey} />
      <button class="primary" onclick={add}>add</button>
    </div>

    {#if otherItems.length > 0}
      <button class="fixall" onclick={fixAll} disabled={fixingAll}>
        {#if fixingAll}
          ✨ categorizing {otherItems.length}…
        {:else}
          ✨ fix all uncategorized ({otherItems.length})
        {/if}
      </button>
    {/if}

    {#if items.length === 0}
      <p class="empty">empty list — add items above</p>
    {:else if activeItems.length === 0}
      <p class="empty">all done — nice</p>
    {:else}
      {#each grouped as g (g.cat)}
        <div class="group">
          <div class="group-head">
            <span class="cat-ico">{CATEGORY_ICONS[g.cat]}</span>
            <span class="cat-name">{g.cat}</span>
            <span class="cat-count">{g.items.length}</span>
          </div>
          <ul class="items">
            {#each g.items as it (it.id)}
              <li
                class:bought={it.bought}
                class:missing={it.missing}
                class:expanded={expandedId === it.id}
              >
                <button
                  class="check"
                  class:on={it.bought}
                  class:x={it.missing}
                  onclick={() => toggle(it)}
                  aria-label={it.bought ? 'mark unbought' : 'mark bought'}
                ></button>
                <button
                  type="button"
                  class="body"
                  onclick={() => (expandedId = expandedId === it.id ? null : it.id)}
                  aria-label="toggle actions"
                >
                  <span class="text">{it.text}</span>
                  {#if it.missing}<span class="missing-tag">✗ can't find</span>{/if}
                  <span class="body-spacer"></span>
                  {#if editingCountFor === it.id}
                    <span class="qty-edit" onclick={(e) => e.stopPropagation()}>
                      <input
                        class="cnt"
                        type="number"
                        step="0.1"
                        min="0.1"
                        bind:value={countDraft}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') commitCount(it);
                          if (e.key === 'Escape') cancelCount();
                        }}
                        onblur={() => commitCount(it)}
                      />
                      <input
                        class="unit"
                        placeholder="unit"
                        bind:value={unitDraft}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') commitCount(it);
                          if (e.key === 'Escape') cancelCount();
                        }}
                        onblur={() => commitCount(it)}
                      />
                    </span>
                  {:else if hasQty(it.count, it.unit)}
                    <span
                      class="qty"
                      class:fuzzy={isFuzzy(it.count, it.unit)}
                      role="button"
                      tabindex="0"
                      onclick={(e) => {
                        e.stopPropagation();
                        startCountEdit(it);
                      }}
                      title="edit quantity"
                    >
                      {formatQty(it.count, it.unit)}
                    </span>
                  {:else}
                    <span
                      class="qty-empty"
                      role="button"
                      tabindex="0"
                      onclick={(e) => {
                        e.stopPropagation();
                        startCountEdit(it);
                      }}
                      title="add quantity"
                    >
                      +
                    </span>
                  {/if}
                </button>
                <button class="icon-btn del" onclick={() => del(it)} aria-label="delete">✕</button>

                <div class="actions">
                  <button
                    class="miss-btn"
                    class:on={it.missing}
                    onclick={() => toggleMissing(it)}
                  >
                    {#if it.missing}✓ found{:else}✗ can't find{/if}
                  </button>
                  <select
                    class="cat-pick"
                    value={it.category}
                    onchange={(e) => setCategory(it, (e.target as HTMLSelectElement).value as Category)}
                  >
                    {#each CATEGORIES as c}
                      <option value={c}>{c}</option>
                    {/each}
                  </select>
                  <button class="icon-btn" onclick={() => reAi(it)} aria-label="re-categorize">✨</button>
                  <button class="icon-btn" onclick={() => openReplace(it)} aria-label="suggest replacement">⇄</button>
                </div>

                {#if replaceFor === it.id}
                  <div class="replace-pop" role="dialog">
                    <div class="replace-head">
                      <span>replacements for <b>{it.text}</b></span>
                      <button class="icon-btn" onclick={closeReplace} aria-label="close">✕</button>
                    </div>
                    {#if replaceLoading}
                      <p class="status saving">thinking…</p>
                    {:else if replaceError}
                      <p class="status error">{replaceError}</p>
                    {:else}
                      <ul class="suggestions">
                        {#each replaceSuggestions as s}
                          <li>
                            <button class="sug-btn" onclick={() => applyReplacement(it, s.text)}>
                              <span class="sug-text">{s.text}</span>
                              {#if s.reason}<span class="sug-reason">{s.reason}</span>{/if}
                            </button>
                          </li>
                        {/each}
                      </ul>
                    {/if}
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    {/if}

    {#if boughtList.length > 0}
      <div class="done-section">
        <button type="button" class="done-toggle" onclick={() => (showDone = !showDone)}>
          <span class="done-chev">{showDone ? '▾' : '▸'}</span>
          <span class="done-label">DONE</span>
          <span class="done-count">{boughtList.length}</span>
        </button>
        {#if showDone}
          <ul class="done-items">
            {#each sortedBought as it (it.id)}
              <li>
                <button
                  class="check on"
                  onclick={() => toggle(it)}
                  aria-label="uncheck"
                ></button>
                <span class="done-text">
                  {it.text}
                  {#if hasQty(it.count, it.unit)}<span class="done-qty">{formatQty(it.count, it.unit)}</span>{/if}
                </span>
                {#if it.bought_at}
                  <span class="done-when">{relative(it.bought_at)}</span>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  {:else}
    <p class="empty">loading…</p>
  {/if}
</section>
