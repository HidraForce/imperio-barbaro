export async function adminAPI(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'same-origin', cache: 'no-store', ...options,
    headers: { 'Content-Type': 'application/json', 'X-Clanboard-Request': '1', ...options.headers },
  })
  const result = await response.json().catch(() => null)
  if (!response.ok) {
    const error = new Error(response.status === 401 ? 'Entre novamente no painel.' : result?.error || 'Não foi possível concluir a operação.')
    error.status = response.status
    throw error
  }
  return result
}
