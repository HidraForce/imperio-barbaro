<script setup>
defineProps({ name: { type: String, default: 'Shield' } })
const paths = {
  Overview: 'M3 3h18v18H3z M3 9h18 M9 9v12 M15 9v12 M3 15h18',
  Raids: 'M4 21V9h4V5h8v4h4v12 M3 21h18 M10 21v-6h4v6 M8 9h8 M10 5V2h4v3',
  CWL: 'M5 3h14v5a7 7 0 0 1-14 0z M5 5H2v3a4 4 0 0 0 4 4 M19 5h3v3a4 4 0 0 1-4 4 M12 15v6 M7 21h10',
  War: 'M3 3l6 2 11 11-4 4L5 9z M14 18l4-4 M17 19l3 3 M21 3l-6 2-3 3 M9 13l-5 5 M6 14l4 4 M7 19l-3 3',
  Roster: 'M9 3a4 4 0 1 1 0 8 4 4 0 0 1 0-8 M2 21v-4a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v4 M17 3a4 4 0 0 1 0 8 M18 13a5 5 0 0 1 4 5v3',
  Contacts: 'M5 3h15v18H5z M2 7h5 M2 12h5 M2 17h5 M10 7h6 M10 12h6 M10 17h4',
  Leadership: 'M3 3h18v18H3z M3 8h18 M7 12h4 M7 16h4 M15 12h2 M15 16h2',
  Rules: 'M4 3h16v18H4z M8 7h8 M8 12h8 M8 17h5',
  Moderation: 'M12 3l10 18H2z M12 9v5 M12 17v1',
  Shield: 'M12 2l9 4v7c0 4-4 7-9 9-5-2-9-5-9-9V6z M12 6v11 M7 11h10',
}
</script>
<template><svg class="ui-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linejoin="miter" aria-hidden="true"><path :d="paths[name]||paths.Shield"/></svg></template>
