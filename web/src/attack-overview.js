const tagKey = tag => (tag || '').replace(/^#/, '').toUpperCase()

function neutral(label) { return { tone: 'neutral', label, pending: false } }

function attackStatus(used, total, ended) {
  const pending = used < total
  return {
    tone: pending ? 'missing' : 'complete', pending,
    label: `${used}/${total} · ${pending ? (ended ? 'Não realizados' : 'Pendentes') : 'Concluídos'}`,
  }
}

function battleStatus(member, battle, loading, unavailable, fallback) {
  if (loading) return neutral('Carregando…')
  if (unavailable || !battle) return neutral('Sem dados')
  if (battle.state === 'notInWar') return neutral('Sem guerra')
  if (!member) return neutral('Não escalado')
  if (battle.state === 'preparation') return neutral('Em preparação')
  if (!['inWar', 'warEnded'].includes(battle.state)) return neutral('Aguardando batalha')
  return attackStatus(member.attacks?.length || 0, battle.attacksPerMember || fallback, battle.state === 'warEnded')
}

export function buildAttackOverview({ members = [], raidRecord, war, leagueWar, leagueMembers = [], loading = false, leagueLoading = false, warError = '', leagueError = '', now = Date.now() }) {
  const raids = new Map()
  const roster = new Set((raidRecord?.roster || []).map(m => tagKey(m.tag)))
  const warPlayers = new Map((war?.clan?.members || []).map(m => [tagKey(m.tag), m]))
  const leaguePlayers = new Map(leagueMembers.map(m => [tagKey(m.tag), m]))
  const players = new Map()
  for (const m of [...(raidRecord?.roster || []), ...(raidRecord?.season?.members || []), ...(war?.clan?.members || []), ...leagueMembers, ...members]) {
    players.set(tagKey(m.tag), m)
  }
  for (const m of raidRecord?.season?.members || []) raids.set(tagKey(m.tag), m)
  const raidEnded = raidRecord?.season?.state === 'ended' || parseClashDate(raidRecord?.season?.endTime) <= now
  const current = new Set(members.map(m => tagKey(m.tag)))
  return [...players.entries()].map(([key, member]) => {
    let raid = neutral(loading ? 'Carregando…' : 'Sem dados')
    if (!loading && raidRecord) {
      const attacker = raids.get(key)
      if (!attacker && !roster.has(key)) raid = neutral('Fora da lista registrada')
      else {
        const used = attacker?.attacks || 0
        const total = attacker?.attackLimit == null ? (used === 0 ? 5 : null) : attacker.attackLimit + (attacker.bonusAttackLimit || 0)
        raid = total == null ? neutral(`${used} ataques · limite indisponível`) : attackStatus(used, total, raidEnded)
      }
    }
    const statuses = {
      raids: raid,
      war: battleStatus(warPlayers.get(key), war, loading, warError, 2),
      cwl: battleStatus(leaguePlayers.get(key), leagueWar, loading || leagueLoading, leagueError, 1),
    }
    return { tag: member.tag, name: member.name, current: current.has(key), statuses, pendingCount: Object.values(statuses).filter(s => s.pending).length }
  }).sort((a, b) => b.pendingCount - a.pendingCount || a.name.localeCompare(b.name, 'pt-BR'))
}

function parseClashDate(value) {
  return Date.parse((value || '').replace(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})\.000Z$/, '$1-$2-$3T$4:$5:$6Z'))
}
