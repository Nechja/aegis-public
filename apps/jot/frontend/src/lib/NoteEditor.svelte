<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorState } from '@codemirror/state';
  import {
    EditorView,
    keymap,
    lineNumbers,
    highlightActiveLine,
    drawSelection,
    placeholder as placeholderExt
  } from '@codemirror/view';
  import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
  import { bracketMatching, indentOnInput, syntaxHighlighting, HighlightStyle } from '@codemirror/language';
  import { markdown } from '@codemirror/lang-markdown';
  import { tags as t } from '@lezer/highlight';

  type Props = {
    value: string;
    placeholder?: string;
    oninput?: (v: string) => void;
    onblur?: () => void;
  };

  let { value = $bindable(''), placeholder = 'write…', oninput, onblur }: Props = $props();

  let host: HTMLDivElement;
  let view: EditorView | null = null;
  let applyingExternal = false;

  // Aegis neon highlight style
  const aegisHighlight = HighlightStyle.define([
    { tag: t.heading1, color: 'var(--neon-pink)', fontWeight: '700', textShadow: '0 0 6px rgba(215,135,215,0.4)' },
    { tag: t.heading2, color: 'var(--neon-pink)', fontWeight: '600' },
    { tag: [t.heading3, t.heading4, t.heading5, t.heading6], color: 'var(--neon-dusty)', fontWeight: '600' },
    { tag: t.strong, color: 'var(--neon-peach)', fontWeight: '700' },
    { tag: t.emphasis, color: 'var(--neon-cyan)', fontStyle: 'italic' },
    { tag: t.strikethrough, color: 'var(--text-muted)', textDecoration: 'line-through' },
    { tag: t.link, color: 'var(--neon-cyan)', textDecoration: 'underline' },
    { tag: t.url, color: 'var(--neon-cyan)' },
    { tag: t.monospace, color: 'var(--neon-green)', background: 'rgba(95,175,0,0.08)' },
    { tag: t.quote, color: 'var(--text-secondary)', fontStyle: 'italic' },
    { tag: t.list, color: 'var(--neon-orange)' },
    { tag: t.meta, color: 'var(--text-muted)' },
    { tag: t.contentSeparator, color: 'var(--neon-dusty)' }
  ]);

  const aegisTheme = EditorView.theme(
    {
      '&': {
        color: 'var(--text-primary)',
        backgroundColor: 'var(--bg-inset)',
        fontFamily: 'var(--font)',
        fontSize: '14px',
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius)',
        minHeight: '320px',
        height: '100%'
      },
      '&.cm-focused': {
        outline: 'none',
        borderColor: 'var(--neon-cyan)',
        boxShadow: 'var(--glow-cyan)'
      },
      '.cm-scroller': {
        fontFamily: 'var(--font)',
        lineHeight: '1.6',
        padding: '6px 0'
      },
      '.cm-content': {
        caretColor: 'var(--neon-cyan)',
        padding: '6px 4px'
      },
      '.cm-line': {
        padding: '0 10px'
      },
      '.cm-cursor, .cm-dropCursor': {
        borderLeftColor: 'var(--neon-pink)',
        borderLeftWidth: '2px'
      },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
        backgroundColor: 'rgba(215, 135, 215, 0.25)'
      },
      '.cm-activeLine': {
        backgroundColor: 'rgba(215, 135, 215, 0.04)'
      },
      '.cm-gutters': {
        backgroundColor: 'var(--bg-card)',
        color: 'var(--text-muted)',
        border: 'none',
        borderRight: '1px solid var(--border-default)',
        fontFamily: 'var(--font)'
      },
      '.cm-activeLineGutter': {
        backgroundColor: 'var(--bg-card-hover)',
        color: 'var(--neon-pink)'
      },
      '.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
        color: 'var(--neon-peach)',
        backgroundColor: 'rgba(255,175,95,0.1)',
        outline: '1px solid var(--neon-peach)'
      },
      '.cm-placeholder': {
        color: 'var(--text-muted)',
        fontStyle: 'italic'
      }
    },
    { dark: true }
  );

  const mobile = () => window.matchMedia('(max-width: 640px)').matches;

  function makeState(doc: string) {
    const extensions = [
      history(),
      drawSelection(),
      highlightActiveLine(),
      bracketMatching(),
      indentOnInput(),
      EditorView.lineWrapping,
      markdown(),
      syntaxHighlighting(aegisHighlight),
      aegisTheme,
      placeholderExt(placeholder),
      keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
      EditorView.updateListener.of((u) => {
        if (!u.docChanged || applyingExternal) return;
        const next = u.state.doc.toString();
        value = next;
        oninput?.(next);
      }),
      EditorView.domEventHandlers({
        blur: () => {
          onblur?.();
          return false;
        }
      }),
      EditorState.tabSize.of(2)
    ];
    if (!mobile()) extensions.push(lineNumbers());
    return EditorState.create({ doc, extensions });
  }

  onMount(() => {
    view = new EditorView({
      state: makeState(value ?? ''),
      parent: host
    });
  });

  // watch for external value changes (e.g. switching notes) and sync into the editor
  $effect(() => {
    if (!view) return;
    const current = view.state.doc.toString();
    if (current === value) return;
    applyingExternal = true;
    view.dispatch({
      changes: { from: 0, to: current.length, insert: value ?? '' }
    });
    applyingExternal = false;
  });

  export function focus() {
    view?.focus();
  }

  onDestroy(() => {
    view?.destroy();
    view = null;
  });
</script>

<div class="note-cm" bind:this={host}></div>

<style>
  .note-cm {
    width: 100%;
  }
  .note-cm :global(.cm-editor) {
    height: auto;
    min-height: 320px;
  }
</style>
