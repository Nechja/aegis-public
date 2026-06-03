export type Category =
  | 'produce'
  | 'dairy'
  | 'meat'
  | 'pantry'
  | 'frozen'
  | 'bakery'
  | 'beverages'
  | 'snacks'
  | 'household'
  | 'other';

export const CATEGORIES: Category[] = [
  'produce',
  'dairy',
  'meat',
  'bakery',
  'pantry',
  'frozen',
  'beverages',
  'snacks',
  'household',
  'other'
];

export const CATEGORY_ICONS: Record<Category, string> = {
  produce: '◐',
  dairy: '◒',
  meat: '◓',
  bakery: '◔',
  pantry: '◕',
  frozen: '❄',
  beverages: '◍',
  snacks: '◉',
  household: '◎',
  other: '·'
};

export type Item = {
  id: number;
  list_id: number;
  text: string;
  category: Category;
  count: number;
  unit: string;
  bought: boolean;
  missing: boolean;
  sort_idx: number;
  created_at: string;
  bought_at?: string | null;
};

export type ListStatus = 'active' | 'archived' | 'template';

export type List = {
  id: number;
  name: string;
  status: ListStatus;
  tags: string[];
  created_at: string;
  updated_at: string;
  completed_at?: string | null;
  items?: Item[];
  item_count?: number;
  cheers?: Record<string, string[]>;
};

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  listLists: (status: ListStatus = 'active') =>
    fetch(`/api/lists?status=${status}`).then(json<List[]>),
  getList: (id: number) => fetch(`/api/lists/${id}`).then(json<List>),
  createList: (name: string) =>
    fetch('/api/lists', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    }).then(json<List>),
  renameList: (id: number, name: string) =>
    fetch(`/api/lists/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    }).then(json<List>),
  deleteList: (id: number) => fetch(`/api/lists/${id}`, { method: 'DELETE' }).then(json<void>),
  resetList: (id: number) => fetch(`/api/lists/${id}/reset`, { method: 'POST' }).then(json<void>),
  duplicateList: (id: number, name?: string, status: ListStatus = 'active') =>
    fetch(`/api/lists/${id}/duplicate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name ?? 'copy', status })
    }).then(json<List>),

  setListStatus: (id: number, status: ListStatus) =>
    fetch(`/api/lists/${id}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status })
    }).then(json<List>),

  setListTags: (id: number, tags: string[]) =>
    fetch(`/api/lists/${id}/tags`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tags })
    }).then(json<List>),

  createItem: (
    list_id: number,
    text: string,
    opts?: { category?: Category; count?: number; unit?: string }
  ) =>
    fetch('/api/items', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        list_id,
        text,
        category: opts?.category ?? '',
        count: opts?.count ?? 0,
        unit: opts?.unit ?? ''
      })
    }).then(json<Item>),
  patchItem: (
    id: number,
    patch: {
      text?: string;
      category?: Category;
      count?: number;
      unit?: string;
      bought?: boolean;
      missing?: boolean;
    }
  ) =>
    fetch(`/api/items/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    }).then(json<Item>),
  deleteItem: (id: number) => fetch(`/api/items/${id}`, { method: 'DELETE' }).then(json<void>),

  importList: (name: string, markdown: string) =>
    fetch('/api/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, markdown })
    }).then(json<List>),

  aiCategorize: (text: string) =>
    fetch('/api/ai/categorize', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text })
    }).then(json<{ category: Category; source: string }>),

  aiCategorizeBatch: (items: { id: number; text: string }[]) =>
    fetch('/api/ai/categorize-batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items })
    }).then(json<{ id: number; category: Category; source: string }[]>),

  aiReplace: (text: string) =>
    fetch('/api/ai/replace', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text })
    }).then(json<{ suggestions: { text: string; reason: string }[] }>),

  generateListCheers: (listId: number) =>
    fetch(`/api/lists/${listId}/cheers/generate`, { method: 'POST' }).then(
      json<{ cheers: Record<string, string[]> }>
    )
};

export function formatQty(count: number, unit: string): string {
  if (count === 1 && !unit) return '';
  if (count === 0 && unit) return unit; // fuzzy quantifier
  const n = formatNumber(count);
  return unit ? `${n} ${unit}` : `×${n}`;
}

export function hasQty(count: number, unit: string): boolean {
  return !(count === 1 && !unit);
}

export function isFuzzy(count: number, unit: string): boolean {
  return count === 0 && !!unit;
}

function formatNumber(f: number): string {
  if (Number.isInteger(f)) return String(f);
  if (f === 0.25) return '1/4';
  if (f === 0.5) return '1/2';
  if (f === 0.75) return '3/4';
  if (f === 1.5) return '1 1/2';
  return f.toFixed(2).replace(/\.?0+$/, '');
}
