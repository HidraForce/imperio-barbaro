export function csvCell(value) {
  let text = String(value ?? '')
  // Nomes vindos da API também podem ser interpretados como fórmulas em planilhas.
  if (typeof value !== 'number' && (/^[\s]*[=+\-@]/.test(text) || /^[\t\r\n]/.test(text))) text = "'" + text
  return '"' + text.replaceAll('"', '""') + '"'
}
