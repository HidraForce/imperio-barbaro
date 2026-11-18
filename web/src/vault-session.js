const maximumLifetime = 10 * 60 * 1000

export function createVaultSession(path) {
  const key = 'clanboard:vault:' + path

  function clear() {
    try { globalThis.sessionStorage?.removeItem(key) } catch {}
  }

  function restore() {
    try {
      const session = JSON.parse(globalThis.sessionStorage?.getItem(key) || 'null')
      if (!session) return null
      const lifetime = session.expiresAt - Date.now()
      if (typeof session.token === 'string' && session.token && Number.isFinite(session.expiresAt) && lifetime > 0 && lifetime <= maximumLifetime) return session
    } catch {}
    clear()
    return null
  }

  function save(result) {
    const expiresAt = Math.min(Date.parse(result.expiresAt), Date.now() + maximumLifetime)
    if (typeof result.token !== 'string' || !result.token || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
      throw Error('O desbloqueio expirou. Tente novamente.')
    }
    const session = { token: result.token, expiresAt }
    try { globalThis.sessionStorage?.setItem(key, JSON.stringify(session)) } catch {}
    return session
  }

  return { clear, restore, save }
}
