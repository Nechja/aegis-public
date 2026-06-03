import type { ListStatus } from './api';

export type Route =
  | { name: 'lists'; status: ListStatus }
  | { name: 'detail'; id: number };

function parsePath(path: string): Route {
  const detail = path.match(/^\/l\/(\d+)\/?$/);
  if (detail) return { name: 'detail', id: parseInt(detail[1], 10) };
  if (path === '/archive' || path === '/archive/') return { name: 'lists', status: 'archived' };
  if (path === '/templates' || path === '/templates/') return { name: 'lists', status: 'template' };
  return { name: 'lists', status: 'active' };
}

function pathOf(r: Route): string {
  if (r.name === 'detail') return `/l/${r.id}`;
  if (r.status === 'archived') return '/archive';
  if (r.status === 'template') return '/templates';
  return '/';
}

export const route = $state<{ current: Route }>({
  current: parsePath(typeof location !== 'undefined' ? location.pathname : '/')
});

export function navigate(to: Route, opts: { replace?: boolean } = {}) {
  const nextPath = pathOf(to);
  if (typeof history !== 'undefined' && nextPath !== location.pathname) {
    if (opts.replace) history.replaceState(null, '', nextPath);
    else history.pushState(null, '', nextPath);
  }
  route.current = to;
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => {
    route.current = parsePath(location.pathname);
  });
}

export function shareLink(r: Route): string {
  return `${location.origin}${pathOf(r)}`;
}
