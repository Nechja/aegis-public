export type Note = {
  id: number;
  title: string;
  body: string;
  created_at: string;
  updated_at: string;
};

export type TodoList = {
  id: number;
  title: string;
  created_at: string;
  updated_at: string;
};

export type Todo = {
  id: number;
  list_id?: number;
  text: string;
  done: boolean;
  created_at: string;
  updated_at: string;
};

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  listNotes: () => fetch('/api/notes').then(json<Note[]>),
  createNote: (title: string, body: string) =>
    fetch('/api/notes', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, body })
    }).then(json<Note>),
  updateNote: (id: number, title: string, body: string) =>
    fetch(`/api/notes/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title, body })
    }).then(json<Note>),
  deleteNote: (id: number) =>
    fetch(`/api/notes/${id}`, { method: 'DELETE' }).then(json<void>),

  listTodoLists: () => fetch('/api/todo-lists').then(json<TodoList[]>),
  createTodoList: (title: string) =>
    fetch('/api/todo-lists', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title })
    }).then(json<TodoList>),
  updateTodoList: (id: number, title: string) =>
    fetch(`/api/todo-lists/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title })
    }).then(json<TodoList>),
  deleteTodoList: (id: number) =>
    fetch(`/api/todo-lists/${id}`, { method: 'DELETE' }).then(json<void>),

  listTodos: (listId?: number) => {
    const url = listId != null ? `/api/todos?list_id=${listId}` : '/api/todos';
    return fetch(url).then(json<Todo[]>);
  },
  createTodo: (text: string, listId?: number) =>
    fetch('/api/todos', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text, list_id: listId })
    }).then(json<Todo>),
  patchTodo: (id: number, patch: { text?: string; done?: boolean; list_id?: number }) =>
    fetch(`/api/todos/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    }).then(json<Todo>),
  deleteTodo: (id: number) =>
    fetch(`/api/todos/${id}`, { method: 'DELETE' }).then(json<void>)
};
