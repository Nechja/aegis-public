<script lang="ts">
  import { api, type List } from './api';

  type Props = {
    oncreated: (l: List) => void;
    oncancel: () => void;
  };
  let { oncreated, oncancel }: Props = $props();

  let name = $state('imported list');
  let md = $state('');
  let busy = $state(false);
  let err = $state('');

  const placeholder = `# shopping list
- 2 lbs chicken thighs
- 1 gallon milk
- apples
- cilantro
- tortillas
- black beans
`;

  async function save() {
    if (busy) return;
    if (md.trim() === '') {
      err = 'paste some markdown first';
      return;
    }
    err = '';
    busy = true;
    try {
      const g = await api.importList(name.trim() || 'imported list', md);
      oncreated(g);
    } catch {
      err = 'import failed — try again';
    } finally {
      busy = false;
    }
  }
</script>

<div class="import">
  <div class="import-head">
    <span class="label">paste markdown · ai will categorize</span>
    <button class="ghost" onclick={oncancel}>cancel</button>
  </div>
  <input class="import-name" placeholder="list name" bind:value={name} />
  <textarea class="import-md" {placeholder} bind:value={md}></textarea>
  <div class="import-foot">
    {#if err}<span class="status error">{err}</span>{/if}
    {#if busy}<span class="status saving">categorizing…</span>{/if}
    <span class="grow"></span>
    <button class="primary" onclick={save} disabled={busy}>save list</button>
  </div>
</div>
